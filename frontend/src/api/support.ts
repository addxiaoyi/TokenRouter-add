import { apiClient } from './client'

// support.ts defines private support conversations and their role-scoped API.
// User endpoints always derive ownership from authentication. Administrator paths
// expose workflow and notification management, while client UUIDs remain stable
// across retries so failed HTTP responses do not duplicate saved submissions.

// TicketStatus identifies the supported conversation workflow states.
export type TicketStatus = 'open' | 'in_progress' | 'resolved' | 'closed'

// SupportTicket describes one conversation without exposing account credentials.
export interface SupportTicket {
  id: number // Conversation record identifier.
  user_id: number // Owning account identifier.
  username: string // Owning account display name.
  subject: string // Plain text conversation title.
  status: TicketStatus // Current workflow state.
  version: number // Revision required for status updates.
  created_at: string // ISO submission timestamp.
  updated_at: string // ISO latest change timestamp.
  resolved_at?: string // ISO completion timestamp when completed.
  legacy_id?: number // Original station ticket identifier.
  message_count: number // Persisted conversation message count.
}

// SupportMessage preserves one immutable participant reply and its submission role.
export interface SupportMessage {
  id: number // Reply record identifier.
  author_id?: number // Author account when retained.
  author: string // Public author display name.
  is_staff: boolean // Staff identity at submission.
  body: string // Plain text message content.
  created_at: string // ISO message submission timestamp.
}

// SupportThread combines current ticket state with a chronological reply page.
export interface SupportThread {
  ticket: SupportTicket // Current conversation state.
  messages: SupportMessage[] // Ordered immutable replies.
  has_more: boolean // Another reply page exists.
  next_id: number // Cursor following the last returned reply.
}

// SupportPage describes the bounded list returned for a validated ticket filter.
export interface SupportPage {
  items: SupportTicket[] // Matching conversation records.
  total: number // Total matching records.
  page: number // One-based page number.
  page_size: number // Requested maximum page length.
}

// SupportSettings contains staff recipients and durable email delivery counts.
export interface SupportSettings {
  recipients: string[] // Addresses for new owner messages.
  delivery: { pending: number; failed: number; delivered: number } // Notification attempt totals.
}

// supportAPI binds the route scope once for all conversation operations.
// The server independently authorizes every request; selecting a staff path here
// does not grant permission. Callers retain submission UUIDs until success.
export function supportAPI(staff: boolean) {
  const base = staff ? '/admin/support-tickets' : '/support-tickets'
  return {
    // list retrieves a bounded page using literal search and workflow filters.
    async list(params: { page: number; status: string; search: string }) {
      return (await apiClient.get<SupportPage>(base, { params })).data
    },
    // thread retrieves the next chronological message page for one authorized ticket.
    async thread(id: number, after = 0) {
      return (await apiClient.get<SupportThread>(`${base}/${id}`, { params: { after_id: after } }))
        .data
    },
    // create submits an owned conversation with its stable retry identity.
    async create(subject: string, message: string, clientID: string) {
      return (
        await apiClient.post<SupportThread>('/support-tickets', {
          subject,
          message,
          client_id: clientID,
        })
      ).data
    },
    // reply appends one message while retaining previous conversation history.
    async reply(id: number, message: string, clientID: string) {
      await apiClient.post(`${base}/${id}/replies`, { message, client_id: clientID })
    },
    // status changes workflow state only against the user's last observed revision.
    async status(ticket: SupportTicket, status: TicketStatus) {
      await apiClient.patch(`${base}/${ticket.id}/status`, { status, version: ticket.version })
    },
    // settings reads administrator notification recipients and delivery diagnostics.
    async settings() {
      return (await apiClient.get<SupportSettings>(`${base}/settings`)).data
    },
    // saveSettings replaces validated staff recipients for future notification intents.
    async saveSettings(recipients: string[]) {
      await apiClient.put(`${base}/settings`, { recipients })
    },
    // retryDelivery reschedules exhausted notifications without repeating completed notices.
    async retryDelivery() {
      await apiClient.post(`${base}/notifications/retry`)
    },
  }
}
