package memoryauthor

import "neo-chat/mm-chat/backend/internal/memoryeval"

var regressionCoreSlices = []string{
	"stable_fact",
	"preference_instruction",
	"project_decision",
	"temporal_correction",
	"unrelated_negative",
	"untrusted_source",
	"secret_rejection",
	"scope_isolation",
	"deletion",
	"failure_fallback",
	"multi_hop",
}

var regressionSplits = []splitProfile{
	{name: "development", total: 300, zh: 210, mixed: 60, en: 30, poolSliceMin: 30},
	{name: "validation", total: 100, zh: 70, mixed: 20, en: 10, poolSliceMin: 10},
	{name: "holdout", total: 100, zh: 70, mixed: 20, en: 10, poolSliceMin: 10},
}

var regressionEntities = []string{
	"Atlas", "Borealis", "Cedar", "Driftwood", "Ember",
	"Fjord", "Grove", "Harbor", "Indigo", "Juniper",
	"Kestrel", "Lumen", "Meadow", "Nimbus", "Orchard",
	"Pebble", "Quartz", "Redwood", "Solace", "Tundra",
	"Umber", "Velvet", "Willow", "Xenon", "Yarrow",
}

var regressionSubjectsZH = []string{
	"构建方式", "界面主题", "发布节奏", "测试区域", "检索策略",
	"文档格式", "提醒习惯", "分支命名", "评审顺序", "日志级别",
	"备份窗口", "通知渠道", "缓存策略", "导出格式", "工作时段",
	"错误处理", "搜索范围", "部署区域", "摘要风格", "归档规则",
}

var regressionSubjectsEN = []string{
	"build method", "interface theme", "release cadence", "test region", "retrieval policy",
	"document format", "reminder habit", "branch naming", "review order", "log level",
	"backup window", "notification channel", "cache policy", "export format", "working hours",
	"error handling", "search scope", "deployment region", "summary style", "archive rule",
}

var regressionValuesZH = []string{
	"仅在工作日执行", "采用低对比度配色", "分两阶段发布", "优先使用东部测试区", "先精确匹配再语义召回",
	"输出简洁的 Markdown", "提前一刻钟提醒", "使用短横线命名", "先安全评审再性能评审", "默认记录警告级别",
	"在清晨完成备份", "只通过站内通知", "保留短期缓存", "导出为结构化文本", "避开夜间工作",
	"失败时返回明确原因", "仅搜索当前授权范围", "优先选择邻近区域", "先给结论再给依据", "按月归档",
}

var regressionValuesEN = []string{
	"run only on weekdays", "use a low-contrast palette", "release in two stages", "prefer the eastern test region", "try exact matching before semantic recall",
	"produce concise Markdown", "send a reminder fifteen minutes early", "use hyphenated names", "review security before performance", "record warnings by default",
	"finish backups in the early morning", "use in-app notifications only", "keep a short-lived cache", "export structured text", "avoid night-time work",
	"return an explicit reason on failure", "search only the authorized scope", "prefer a nearby region", "state the conclusion before evidence", "archive monthly",
}

// The v4 semantic profile uses an explicit positionally aligned current/old
// value pair for every subject. Historical v2/v3 keep the modular value
// permutation in newRegressionScenario so their bytes and evidence remain
// immutable.
var regressionSemanticCurrentValuesZH = []string{
	"增量构建", "低对比度配色", "两阶段发布", "东部测试区", "先精确匹配再语义召回",
	"简洁的 Markdown", "提前一刻钟提醒", "短横线命名", "先安全评审再性能评审", "警告日志级别",
	"清晨备份窗口", "站内通知", "短期缓存", "JSON", "避开夜间的工作时段",
	"失败时返回明确原因", "当前授权范围", "邻近部署区域", "先结论后依据", "按月归档",
}

var regressionSemanticOldValuesZH = []string{
	"全量构建", "高对比度配色", "一次性发布", "西部测试区", "仅语义召回",
	"纯文本", "提前半小时提醒", "下划线命名", "先性能评审再安全评审", "信息日志级别",
	"深夜备份窗口", "邮件通知", "长期缓存", "CSV", "允许夜间的工作时段",
	"失败时返回通用错误", "全部可见范围", "远端部署区域", "先依据后结论", "按季度归档",
}

var regressionSemanticCurrentValuesEN = []string{
	"use incremental builds", "use a low-contrast palette", "release in two stages", "prefer the eastern test region", "try exact matching before semantic recall",
	"produce concise Markdown", "send a reminder fifteen minutes early", "use hyphenated names", "review security before performance", "record warnings by default",
	"finish backups in the early morning", "use in-app notifications only", "keep a short-lived cache", "export as JSON", "avoid night-time work",
	"return an explicit reason on failure", "search only the authorized scope", "prefer a nearby region", "state the conclusion before evidence", "archive monthly",
}

var regressionSemanticOldValuesEN = []string{
	"use full rebuilds", "use a high-contrast palette", "release in one stage", "prefer the western test region", "use semantic recall only",
	"produce plain text", "send a reminder thirty minutes early", "use underscored names", "review performance before security", "record informational events by default",
	"finish backups late at night", "use email notifications only", "keep a long-lived cache", "export as CSV", "allow night-time work",
	"return a generic error on failure", "search every visible scope", "prefer a remote region", "state the evidence before the conclusion", "archive quarterly",
}

type regressionDraft struct {
	index    int
	split    string
	language string
	primary  string
	slices   map[string]struct{}
}

type regressionScenario struct {
	draft      regressionDraft
	entity     string
	subjectZH  string
	subjectEN  string
	valueZH    string
	valueEN    string
	oldValueZH string
	oldValueEN string
	scope      memoryeval.Scope
}
