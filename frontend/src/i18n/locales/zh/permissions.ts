// permissions.ts describes the access rules shown in the administrator reference.
// Role and resource keys match the stable identifiers included in catalog exports,
// while the labels explain the scope of each rule in the current language.

export default {
  title: '权限目录',
  description: '查询各角色与凭据可以访问的资源，以及对应的限制。',
  roles: '角色基线',
  readOnly: '只读参考',
  resources: '资源访问规则',
  search: '搜索资源规则',
  noResults: '没有匹配的资源规则。',
  export: '导出 JSON',
  role: {
    user: {
      label: '普通用户',
      scope: '站点账户',
      rule: '使用个人工作台并管理归属本人账户的资源。团队访问取决于当前成员关系。',
    },
    admin: {
      label: '管理员',
      scope: '站点管理',
      rule: '访问管理后台。更改受保护的设置时，可能需要近期完成双因素验证。',
    },
    team_owner: {
      label: '团队所有者',
      scope: '单个团队',
      rule: '管理当前团队的成员、邀请与额度，查看全队用量，并为团队密钥发起的请求付款。',
    },
    team_member: {
      label: '团队成员',
      scope: '单个团队',
      rule: '成员关系有效时，可在成员额度内使用团队资源。团队报表只显示本人的用量。',
    },
  },
  resource: {
    api_keys: {
      label: 'API 密钥',
      rule: '管理本人账户名下的密钥。网关密钥受自身规则、分组准入和额度限制。',
    },
    routing: {
      label: '模型路由',
      rule: '公开分组和账户获授权的专属分组可供使用。网关按每次请求校验密钥和分组策略。',
    },
    usage: {
      label: '用量记录',
      rule: '个人用量按当前账户隔离。团队报表还会检查成员关系与团队角色。全站用量记录在管理后台查看。',
    },
    billing: {
      label: '订单与订阅',
      rule: '用户可查看归属本人账户的订单与权益。全站订单管理需要管理员权限。',
    },
    support: {
      label: '工单',
      rule: '用户可创建、查看和回复本人的工单。管理员可在工单后台查看和回复全站工单。',
    },
    teams: {
      label: '团队',
      rule: '团队操作校验当前成员关系。所有者管理成员，普通成员的操作受本人身份和额度限制。',
    },
    admin: {
      label: '管理后台',
      rule: '服务端检查当前账户的站点管理员角色。敏感设置可能要求管理员在当前会话近期完成双因素验证。',
    },
    mcp: {
      label: 'MCP 管理',
      rule: 'MCP 凭据有独立的授权操作、有效期和撤销状态。服务端还会核对所属账户仍有管理员权限。',
    },
  },
}
