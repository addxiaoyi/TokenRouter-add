CREATE TABLE support_tickets (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    subject VARCHAR(160) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'in_progress', 'resolved', 'closed')),
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,
    legacy_id BIGINT UNIQUE,
    client_id UUID NOT NULL,
    UNIQUE (user_id, client_id)
);
CREATE INDEX support_tickets_owner_updated_idx ON support_tickets(user_id, updated_at DESC, id DESC);
CREATE INDEX support_tickets_status_updated_idx ON support_tickets(status, updated_at DESC, id DESC);

CREATE TABLE support_ticket_messages (
    id BIGSERIAL PRIMARY KEY,
    ticket_id BIGINT NOT NULL REFERENCES support_tickets(id) ON DELETE CASCADE,
    author_id BIGINT REFERENCES users(id),
    is_staff BOOLEAN NOT NULL DEFAULT FALSE,
    body TEXT NOT NULL CHECK (char_length(body) BETWEEN 1 AND 10000),
    client_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (ticket_id, client_id)
);
CREATE INDEX support_ticket_messages_thread_idx ON support_ticket_messages(ticket_id, id);

CREATE TABLE support_ticket_notifications (
    id BIGSERIAL PRIMARY KEY,
    ticket_id BIGINT NOT NULL REFERENCES support_tickets(id) ON DELETE CASCADE,
    message_id BIGINT NOT NULL REFERENCES support_ticket_messages(id) ON DELETE CASCADE,
    recipient TEXT NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    lease_id UUID NOT NULL DEFAULT gen_random_uuid(),
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    UNIQUE (message_id, recipient)
);
CREATE INDEX support_ticket_notifications_pending_idx ON support_ticket_notifications(available_at, id) WHERE delivered_at IS NULL;
