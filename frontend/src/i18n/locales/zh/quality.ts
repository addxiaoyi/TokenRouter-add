// quality.ts provides Chinese assessment labels and operational explanations.
// Complete scores describe only the fixed question set; incomplete responses
// retain a separate status, and administrator controls disclose upstream cost
// before a test starts.
export default {
  title: '模型能力检测',
  description: '查看固定题集的实测结果与历史变化。',
  explanation:
    '8 道固定题目覆盖计算、逻辑、代码和指令遵循。分数仅代表这组题目的表现；网络错误或截断回复不算答错，未完成整套检测不生成分数。',
  search: '搜索模型或公开名称',
  refresh: '刷新',
  manage: '管理检测',
  newTest: '发起检测',
  provider: '提供商',
  model: '模型',
  label: '公开名称（可选）',
  costNotice:
    '每次最多发起 8 个真实上游请求，每题 60 秒、最多请求 1024 个输出 token。费用计入系统模型使用记录；同时只运行一项检测，失败不会自动重发。',
  noTargets: '暂无可检测的 API Key 文本模型。请先配置提供商的模型白名单与原生文本协议。',
  start: '开始检测',
  starting: '提交中…',
  queued: '排队中',
  running: '检测中',
  complete: '已完成',
  incomplete: '未完成',
  progress: '已处理 {done} / {total} 题',
  details: '查看题目结果',
  empty: '暂无检测结果',
  delta: '较上次 {value} 分',
  retest: '分数下降，建议复测',
  pass: '通过',
  fail: '答错',
  error: '请求未完成',
  detailNote: '只展示题目结果和耗时，不公开提供商凭据或原始回答。',
  baseline: '比较基线：{time}',
  loadFailed: '检测结果加载失败，请重试。',
  progressFailed: '进度读取失败，正在重试。',
  startFailed: '检测未能提交，可能已有任务运行或配置已变化。请刷新后重试。',
}
