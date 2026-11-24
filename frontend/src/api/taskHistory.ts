import { apiClient } from './client'

// taskHistory.ts reads safe summaries from the authenticated panel. A browser
// session is enough to list the user's tasks across keys. The gateway still
// requires original key ownership for inspecting job items and downloading
// results. The server independently authorizes the administrator branch.

// TaskStatus names the stored states retained by batch history.
export type TaskStatus =
  | 'created'
  | 'uploading'
  | 'submitted'
  | 'running'
  | 'indexing'
  | 'settling'
  | 'completed'
  | 'failed'
  | 'cancelled'
  | 'output_deleted'

// TaskSummary contains only the fields displayed by the read-only panel.
export interface TaskSummary {
  id: string // Public batch identifier.
  user_id?: number // Administrator-only submitter ID.
  model: string // Client model or empty for older rows.
  status: TaskStatus // Current batch lifecycle state.
  item_count: number // Number of requested items.
  success_count: number // Number of successful items.
  failure_count: number // Number of failed items.
  cancelled_count: number // Number of cancelled items.
  created_at: string // ISO creation timestamp.
  finished_at?: string // ISO completion timestamp when recorded.
}

// TaskHistoryPage returns one list and the cursor for the following page.
export interface TaskHistoryPage {
  items: TaskSummary[] // Visible tasks in descending creation order.
  next_cursor?: string // Exclusive cursor for more tasks.
}

// TaskHistoryFilter narrows reads by exact job status, ID, or submitter.
export interface TaskHistoryFilter {
  batch_id?: string // Exact public batch identifier.
  status?: TaskStatus // Exact persisted batch status.
  user_id?: string // Administrator-only positive integer filter.
  cursor?: string // Cursor returned by an earlier filtered page.
}

// readTaskHistory reads one page after the server validates route permissions.
// Ignore interrupted requests when switching accounts, applying another filter,
// or leaving the page. The response contains no execution payloads.
export async function readTaskHistory(
  scope: 'account' | 'admin',
  params: TaskHistoryFilter,
  signal: AbortSignal,
): Promise<TaskHistoryPage> {
  const path = scope === 'admin' ? '/admin/task-history' : '/task-history'
  return (await apiClient.get<TaskHistoryPage>(path, { params, signal })).data
}
