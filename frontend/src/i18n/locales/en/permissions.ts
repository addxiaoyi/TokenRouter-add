// permissions.ts describes the access rules shown in the administrator reference.
// Role and resource keys match the stable identifiers included in catalog exports,
// while the labels explain the scope of each rule in the current language.

export default {
  title: 'Permissions',
  description: 'See which roles and credentials can access each part of the service.',
  roles: 'Role baselines',
  readOnly: 'Read-only reference',
  resources: 'Resource access',
  search: 'Search resource rules',
  noResults: 'No resource rules match this search.',
  export: 'Export JSON',
  role: {
    user: {
      label: 'User',
      scope: 'Site account',
      rule: 'Use the personal workspace and manage resources owned by your account. Team access depends on current membership.',
    },
    admin: {
      label: 'Administrator',
      scope: 'Site administration',
      rule: 'Use the administration console. Protected settings can require recent two-factor verification.',
    },
    team_owner: {
      label: 'Team owner',
      scope: 'One team',
      rule: 'Manage members, invitations, and limits. View team usage and pay for requests made with team keys.',
    },
    team_member: {
      label: 'Team member',
      scope: 'One team',
      rule: 'Use team resources while membership is active and within member limits. Team usage reports show your own usage.',
    },
  },
  resource: {
    api_keys: {
      label: 'API keys',
      rule: 'Manage keys owned by your account. A gateway key can make requests under its configured rules, group access, and quota.',
    },
    routing: {
      label: 'Model routing',
      rule: 'Access public groups and exclusive groups granted to your account. The gateway checks the key and group policy on each request.',
    },
    usage: {
      label: 'Usage records',
      rule: 'Personal usage is scoped to the signed-in account. Team reports also check membership and role. The admin console provides site-wide usage records.',
    },
    billing: {
      label: 'Orders and subscriptions',
      rule: 'View orders and entitlements tied to your account. Site-wide order administration requires administrator access.',
    },
    support: {
      label: 'Support tickets',
      rule: 'Create, read, and reply to your own tickets. Administrators can read and reply to tickets across the site from the support console.',
    },
    teams: {
      label: 'Teams',
      rule: 'Team operations check the current membership. Owners manage team membership while ordinary members act under their own account and limits.',
    },
    admin: {
      label: 'Administration console',
      rule: 'The server checks the current site administrator role. Sensitive settings can require an administrator session with recent two-factor verification.',
    },
    mcp: {
      label: 'MCP management',
      rule: 'MCP credentials have their own allowed operations, expiration, and revocation. The server also checks the owner’s current administrator access.',
    },
  },
}
