package web

// Ops 是 REST 与 CLI api 子命令共用的业务动作集（实现于 cmd/ops.go）。
// 所有方法返回可直接放入 Envelope.Data 的值；错误由调用方包成 {"ok":false,"error":...}
type Ops interface {
	Status() (any, error)
	ServerAction(action string) (any, error) // start | stop | restart
	ServerSet(payload []byte) (any, error)   // body: settings.server 段全量
	Apps() (any, error)
	GadgetRules() (any, error)
	GadgetSet(payload []byte) (any, error) // body: {"rules":[...]} 全量替换，成功后尝试 apply
	BinList() (any, error)
	BinSources() (any, error)
	// async=true 时注册进 TaskManager 立即返回 {"task_id":id}；async=false 同步执行返回 Binary
	BinDownload(variant, binType, version string, async bool, tm *TaskManager) (any, error)
	BinImport(binType, path, version string, async bool, tm *TaskManager) (any, error)
	BinRemove(binType, file string) (any, error)
	BinUse(binType, file string) (any, error)
	BinCleanup() (any, error)
	Logs(name string, tail int) (any, error) // name ∈ ctl|server|web
	WebInfo() (any, error)
}
