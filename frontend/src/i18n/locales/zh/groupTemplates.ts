// groupTemplates.ts supplies Chinese labels for the group creation template
// controls. Scope text describes the reused settings and browser storage, and
// feedback distinguishes an applied draft from a created group.
export default {
  title: '分组模板',
  scope: '复用计费倍率、可见范围、会话隔离、OAuth 和隐私设置。模板保存在当前浏览器，按账号区分。',
  unavailable: '当前浏览器无法读取已保存的模板。',
  choose: '选择模板',
  apply: '填入表单',
  remove: '移除模板',
  name: '新模板名称',
  save: '保存模板',
  saved: '模板已保存。',
  applied: '模板已填入表单。请检查设置后创建分组。',
  removed: '模板已移除。',
  invalidPolicy: '请输入有效的计费倍率，再保存模板。',
  duplicateName: '该模板名称已被使用。',
  libraryFull: '最多可保存 {count} 个模板。',
  storageFailed: '无法更新当前浏览器中的模板。',
}
