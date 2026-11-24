-- 319_payment_tax_invoices.sql stores immutable buyer applications and their
-- financial approval evidence before an external invoice request can begin.
-- Payment completion alone does not establish a taxable sale or its invoice
-- timing, so trusted operators record one explicit decision for each order.
-- Buyer-facing APIs cannot grant this eligibility or select seller tax settings.
--
-- Invoice insertion and refund transitions serialize on the same payment row.
-- The winning transaction commits its decision before the competing flow can
-- proceed, and the losing flow observes that state before changing records.
-- Request identity, buyer details, collected CNY cents, merchant association,
-- and the original approval evidence remain fixed through provider recovery.
-- Worker leases can change independently without replacing those saved facts.
CREATE TABLE IF NOT EXISTS payment_tax_invoice_eligibility (
    order_id BIGINT PRIMARY KEY REFERENCES payment_orders(id),
    reference TEXT NOT NULL CHECK (length(btrim(reference)) BETWEEN 1 AND 500),
    approved_by TEXT NOT NULL CHECK (length(btrim(approved_by)) BETWEEN 1 AND 120),
    approved_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ
);

-- One immutable request survives network timeouts and worker restarts. The
-- provider's record ID is stored separately from both the payment and request IDs.
CREATE TABLE IF NOT EXISTS payment_tax_invoices (
    id UUID PRIMARY KEY,
    order_id BIGINT NOT NULL UNIQUE REFERENCES payment_orders(id),
    user_id BIGINT NOT NULL,
    request_id VARCHAR(80) NOT NULL UNIQUE CHECK (request_id ~ '^[A-Za-z0-9_-]{16,80}$'),
    trade_no VARCHAR(64) NOT NULL CHECK (trade_no ~ '^[A-Za-z0-9_-]{1,64}$'),
    amount_cents BIGINT NOT NULL CHECK (amount_cents BETWEEN 1 AND 9007199254740991),
    buyer_type VARCHAR(16) NOT NULL CHECK (buyer_type IN ('INDIVIDUAL','ORGANIZATION')),
    title VARCHAR(160) NOT NULL CHECK (length(btrim(title)) > 0),
    tax_id VARCHAR(20) NOT NULL DEFAULT '',
    email VARCHAR(214) NOT NULL CHECK (octet_length(email) BETWEEN 3 AND 214),
    endpoint TEXT NOT NULL CHECK (endpoint ~ '^https://'),
    merchant_id VARCHAR(80) NOT NULL CHECK (merchant_id ~ '^[A-Za-z0-9_-]{1,80}$'),
    epay_pid VARCHAR(80) NOT NULL,
    provider_instance_id VARCHAR(64) NOT NULL,
    approval_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    status VARCHAR(16) NOT NULL DEFAULT 'UNKNOWN'
        CHECK (status IN ('UNKNOWN','PROCESSING','ISSUED','FAILED','REVERSING','REVERSED')),
    remote_id VARCHAR(36) NOT NULL DEFAULT '',
    invoice_no VARCHAR(64) NOT NULL DEFAULT '',
    error_code VARCHAR(80) NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    next_sync_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    lease_until TIMESTAMPTZ NOT NULL DEFAULT 'epoch',
    CHECK ((buyer_type='INDIVIDUAL' AND tax_id='') OR
           (buyer_type='ORGANIZATION' AND tax_id ~ '^[A-Z0-9]{15,20}$')),
    CHECK (status<>'ISSUED' OR invoice_no ~ '^[A-Za-z0-9_-]{1,64}$'),
    CHECK (status<>'FAILED' OR error_code IN ('invoice_not_submitted','wechat_param_error',
           'wechat_no_auth','wechat_rule_limit','wechat_issue_failed')),
    CHECK (status NOT IN ('UNKNOWN','PROCESSING','FAILED') OR invoice_no=''),
    CHECK (remote_id='' OR remote_id ~ '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$')
);

CREATE INDEX IF NOT EXISTS payment_tax_invoices_due_idx ON payment_tax_invoices
    (endpoint,merchant_id,epay_pid,provider_instance_id,next_sync_at,id)
    WHERE status IN ('UNKNOWN','PROCESSING','REVERSING');

-- Both insertion and refund updates lock payment_orders first. A losing
-- transaction then sees the winner's committed state and cannot start both flows.
CREATE OR REPLACE FUNCTION guard_payment_tax_invoice() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    payment payment_orders%ROWTYPE;
    approval payment_tax_invoice_eligibility%ROWTYPE;
BEGIN
    IF TG_OP='INSERT' THEN
        SELECT * INTO payment FROM payment_orders WHERE id=NEW.order_id FOR UPDATE;
        IF NOT FOUND OR payment.user_id<>NEW.user_id OR payment.status<>'COMPLETED'
           OR payment.paid_at IS NULL OR payment.refund_amount<>0
           OR payment.refund_at IS NOT NULL OR payment.refund_requested_at IS NOT NULL THEN
            RAISE EXCEPTION 'payment is not eligible for a tax invoice' USING ERRCODE='23514';
        END IF;
        IF payment.payment_trade_no<>NEW.trade_no OR payment.pay_amount*100<>NEW.amount_cents
           OR payment.payment_type !~ '^wxpay($|_)'
           OR COALESCE(payment.provider_key,'')<>'easypay'
           OR COALESCE(payment.provider_instance_id,'')<>NEW.provider_instance_id
           OR COALESCE(payment.provider_snapshot->>'currency','')<>'CNY'
           OR COALESCE(payment.provider_snapshot->>'merchant_id','')<>NEW.epay_pid THEN
            RAISE EXCEPTION 'payment identity differs from tax invoice snapshot' USING ERRCODE='23514';
        END IF;
        SELECT * INTO approval FROM payment_tax_invoice_eligibility
            WHERE order_id=NEW.order_id AND revoked_at IS NULL FOR SHARE;
        IF NOT FOUND THEN
            RAISE EXCEPTION 'financial approval is required for tax invoice' USING ERRCODE='23514';
        END IF;
        IF NEW.status<>'UNKNOWN' OR NEW.remote_id<>'' OR NEW.invoice_no<>'' THEN
            RAISE EXCEPTION 'new tax invoice must preserve an unknown outcome' USING ERRCODE='23514';
        END IF;
        NEW.approval_snapshot=jsonb_build_object('reference',approval.reference,
            'approved_by',approval.approved_by,'approved_at',approval.approved_at);
        RETURN NEW;
    END IF;
    IF ROW(NEW.id,NEW.order_id,NEW.user_id,NEW.request_id,NEW.trade_no,NEW.amount_cents,
           NEW.buyer_type,NEW.title,NEW.tax_id,NEW.email,NEW.endpoint,NEW.merchant_id,
           NEW.epay_pid,NEW.provider_instance_id,NEW.approval_snapshot)
       IS DISTINCT FROM
       ROW(OLD.id,OLD.order_id,OLD.user_id,OLD.request_id,OLD.trade_no,OLD.amount_cents,
           OLD.buyer_type,OLD.title,OLD.tax_id,OLD.email,OLD.endpoint,OLD.merchant_id,
           OLD.epay_pid,OLD.provider_instance_id,OLD.approval_snapshot) THEN
        RAISE EXCEPTION 'tax invoice application is immutable' USING ERRCODE='23514';
    END IF;
    IF (OLD.remote_id<>'' AND NEW.remote_id<>OLD.remote_id)
       OR (OLD.invoice_no<>'' AND NEW.invoice_no<>OLD.invoice_no) THEN
        RAISE EXCEPTION 'tax invoice remote identity is immutable' USING ERRCODE='23514';
    END IF;
    IF (OLD.status IN ('FAILED','REVERSED') AND NEW.status<>OLD.status)
       OR (OLD.status='ISSUED' AND NEW.status NOT IN ('ISSUED','REVERSING','REVERSED'))
       OR (OLD.status='REVERSING' AND NEW.status NOT IN ('REVERSING','ISSUED','REVERSED')) THEN
        RAISE EXCEPTION 'tax invoice observation cannot regress' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS payment_tax_invoice_guard ON payment_tax_invoices;
CREATE TRIGGER payment_tax_invoice_guard BEFORE INSERT OR UPDATE ON payment_tax_invoices
    FOR EACH ROW EXECUTE FUNCTION guard_payment_tax_invoice();

-- Unknown outcomes still require issuer reconciliation. Only confirmed failure
-- codes admitted above, or an explicit reversal, release an existing refund lock.
CREATE OR REPLACE FUNCTION guard_payment_tax_invoice_refund() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (NEW.status LIKE '%REFUND%' OR NEW.refund_amount<>0 OR NEW.refund_at IS NOT NULL
        OR NEW.refund_requested_at IS NOT NULL)
       AND EXISTS(SELECT 1 FROM payment_tax_invoices WHERE order_id=OLD.id AND status NOT IN ('FAILED','REVERSED')) THEN
        RAISE EXCEPTION 'resolve tax invoice before refunding payment' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS payment_tax_invoice_refund_guard ON payment_orders;
CREATE TRIGGER payment_tax_invoice_refund_guard
    BEFORE UPDATE OF status,refund_amount,refund_at,refund_requested_at ON payment_orders
    FOR EACH ROW EXECUTE FUNCTION guard_payment_tax_invoice_refund();
