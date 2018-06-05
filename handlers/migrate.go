package handlers

type MigrateHandler struct {
	Handler
}

func (MigrateHandler) Handle(ctx Context) bool {
	if ctx.Update.Message == nil || ctx.Update.Message.MigrateToChatID == 0 {
		return false
	}
	ctx.MigrateChatId(ctx.ChatId(), ctx.Update.Message.MigrateToChatID)

	return true
}

func (MigrateHandler) Name() string {
	return "MigrateHandler"
}
