// quality.ts provides English assessment labels and operational explanations.
// Complete scores describe only the fixed question set; incomplete responses
// retain a separate status, and administrator controls disclose upstream cost
// before a test starts.
export default {
  title: 'Model assessments',
  description: 'Inspect fixed-suite results and historical changes.',
  explanation:
    'Eight fixed questions cover arithmetic, logic, code, and instruction following. Scores describe only this suite. Network failures and truncated responses are not wrong answers; incomplete suites receive no score.',
  search: 'Search model or public label',
  refresh: 'Refresh',
  manage: 'Manage assessments',
  newTest: 'New assessment',
  provider: 'Provider',
  model: 'Model',
  label: 'Public label (optional)',
  costNotice:
    'Each assessment makes up to 8 real upstream requests, with 60 seconds and a requested limit of 1024 output tokens per question. Costs enter system usage records. One assessment runs at a time; failures are not automatically retried.',
  noTargets:
    'No eligible API-key text models. Configure provider model lists and native text protocols first.',
  start: 'Start assessment',
  starting: 'Submitting…',
  queued: 'Queued',
  running: 'Running',
  complete: 'Complete',
  incomplete: 'Incomplete',
  progress: 'Processed {done} / {total} questions',
  details: 'View question results',
  empty: 'No assessment results',
  delta: 'Change: {value} points',
  retest: 'Score declined; consider a retest',
  pass: 'Passed',
  fail: 'Incorrect',
  error: 'Request incomplete',
  detailNote:
    'Only question outcomes and duration are public. Provider credentials and raw answers remain private.',
  baseline: 'Comparison baseline: {time}',
  loadFailed: 'Could not load assessments. Retry the request.',
  progressFailed: 'Could not read progress. Retrying.',
  startFailed:
    'Could not submit the assessment. Another job may be running or configuration changed. Refresh and retry.',
}
