# WheelMaker æž¶æž„é‡è®¾è®¡

## ç›®æ ‡æè¿°

é‡æž„çŽ°æœ‰ WheelMaker ä»£ç åº“ï¼Œä½¿åŒ…å‘½åä¸Ž ACP åè®®è¯­ä¹‰å¯¹é½ï¼Œå¹¶å¼•å…¥æ›´æ¸…æ™°çš„æŠ½è±¡å±‚æ¬¡ã€‚æ ¸å¿ƒå˜æ›´å¦‚ä¸‹ï¼š(1) å°† `internal/acp/` ä¼ è¾“å±‚é‡å‘½åä¸º `internal/agent/acp/`ï¼Œ`Client` æ”¹ä¸º `Conn`ï¼›(2) å°† `agent.Agent` æŽ¥å£æ›¿æ¢ä¸ºå…·ä½“çš„ `agent.Agent` struct ä»¥åŠçª„æŽ¥å£ `agent.Session`ï¼›(3) å¼•å…¥æ–°çš„ `internal/provider/` å±‚ä½œä¸ºæ— çŠ¶æ€è¿žæŽ¥å·¥åŽ‚ï¼›(4) å°† `internal/hub/` é‡å‘½åä¸º `internal/client/`ï¼Œ`Hub` æ”¹ä¸º `Client`ï¼ŒClient åŒæ—¶æŒæœ‰ `session agent.Session`ï¼ˆç”¨äºŽæ—¥å¸¸æ“ä½œï¼Œå¯ mockï¼‰å’Œ `*agent.Agent`ï¼ˆç”¨äºŽ `Switch` ç­‰ï¼‰ï¼›(5) è¿ç§»çŠ¶æ€æŒä¹…åŒ– JSON keyï¼Œä¿æŒå‘åŽå…¼å®¹ï¼›(6) å°† stdin å¾ªçŽ¯ä»Ž `main.go` ç§»å…¥ `client.Client.Run`ã€‚

æ‰€æœ‰çŽ°æœ‰è¡Œä¸ºå¿…é¡»ä¿ç•™ï¼šACP JSON-RPC é€šä¿¡ã€session ç”Ÿå‘½å‘¨æœŸã€fs/terminal/permission å›žè°ƒã€å‘½ä»¤è§£æžã€çŠ¶æ€æŒä¹…åŒ–ï¼Œä»¥åŠå·²æœ‰çš„ 16 ä¸ª `acp/client` å•å…ƒæµ‹è¯•ï¼ˆé‡å‘½ååŽç»§ç»­é€šè¿‡ï¼‰ã€‚

## éªŒæ”¶æ ‡å‡†

éµå¾ª TDD ç†å¿µï¼Œæ¯æ¡æ ‡å‡†å‡åŒ…å«æ­£å‘æµ‹è¯•å’Œè´Ÿå‘æµ‹è¯•ï¼Œä»¥ä¾¿ç¡®å®šæ€§éªŒè¯ã€‚

- AC-1: åŒ…ç»“æž„ä¸Žè§„èŒƒä¸€è‡´ï¼ˆÂ§5ã€Â§12ï¼‰ï¼šæ–°æ–‡ä»¶å­˜åœ¨ï¼Œæ—§æ–‡ä»¶å·²åˆ é™¤ï¼ŒGo æž„å»ºæˆåŠŸã€‚
  - æ­£å‘æµ‹è¯•ï¼ˆæœŸæœ› PASSï¼‰ï¼š
    - `go build ./...` åœ¨é‡æž„åŽæ— é”™è¯¯å®Œæˆã€‚
    - `go vet ./...` æ— æŠ¥å‘Šé—®é¢˜ã€‚
    - `internal/agent/acp/conn.go`ã€`internal/provider/provider.go`ã€`internal/provider/codex/provider.go`ã€`internal/client/client.go` å‡å­˜åœ¨ã€‚
    - `internal/agent/update.go`ã€`internal/agent/permission.go`ã€`internal/agent/terminal.go`ã€`internal/agent/callbacks.go`ã€`internal/agent/session.go`ã€`internal/agent/prompt.go` å‡å­˜åœ¨ã€‚
  - è´Ÿå‘æµ‹è¯•ï¼ˆæœŸæœ› FAILï¼‰ï¼š
    - æ—§ç›®å½• `internal/acp/`ã€`internal/hub/`ã€`internal/agent/codex/` ä¸å†å­˜åœ¨ï¼ˆä»»ä½•å¯¹å®ƒä»¬çš„æ–‡ä»¶å¼•ç”¨å‡å¯¼è‡´æž„å»ºé”™è¯¯ï¼‰ã€‚

- AC-2: `acp.Conn`ï¼ˆä»Ž `acp.Client` é‡å‘½åï¼‰é€šè¿‡å…¨éƒ¨ 16 ä¸ªçŽ°æœ‰å•å…ƒæµ‹è¯•ï¼Œè¿ç§»è‡³ `internal/agent/acp/conn_test.go`ã€‚
  - æ­£å‘æµ‹è¯•ï¼ˆæœŸæœ› PASSï¼‰ï¼š
    - `go test ./internal/agent/acp/...` è¿è¡Œï¼Œå…¨éƒ¨ 16 ä¸ªæµ‹è¯•é€šè¿‡ã€‚
    - è‡ªå¼•ç”¨ mock æ¨¡å¼ï¼ˆ`GO_ACP_MOCK=1`ï¼‰åœ¨é‡å‘½ååŽçš„ `Conn` ç±»åž‹ä¸‹æ­£å¸¸å·¥ä½œã€‚
    - æ‰€æœ‰å·²æµ‹è¯•è¡Œä¸ºæ­£å¸¸ï¼šè¯·æ±‚/å“åº”ã€å¹¶å‘è¯·æ±‚ã€è®¢é˜…ã€å–æ¶ˆã€`OnRequest` å¤„ç†å™¨ã€è¿›ç¨‹é€€å‡ºæ—¶è§£é™¤é˜»å¡žã€‚
  - è´Ÿå‘æµ‹è¯•ï¼ˆæœŸæœ› FAILï¼‰ï¼š
    - å‘å·²å…³é—­çš„ `Conn` å‘é€æ¶ˆæ¯è¿”å›ž errorï¼ˆä¸ panic æˆ–æŒ‚èµ·ï¼‰ã€‚

- AC-3: `agent.Session` æŽ¥å£æ˜¯ `client.Client` ä¾èµ–çš„çª„å¥‘çº¦ï¼›`agent.Agent` struct å®žçŽ°è¯¥æŽ¥å£ã€‚
  - æ­£å‘æµ‹è¯•ï¼ˆæœŸæœ› PASSï¼‰ï¼š
    - å®žçŽ°äº† `agent.Session` çš„ mock struct å¯ä»¥æ³¨å…¥ `client.Client` è€Œä¸å¼•å‘ç±»åž‹æ–­è¨€ panicã€‚
    - `agent.Agent` struct ä½œä¸º `agent.Session` çš„å®žçŽ°å¯ç¼–è¯‘é€šè¿‡ï¼ˆé€šè¿‡ `var _ agent.Session = (*agent.Agent)(nil)` éªŒè¯ï¼‰ã€‚
    - `agent.Agent.AdapterName()` å’Œ `agent.Agent.SessionID()` è¿”å›žæ­£ç¡®çš„å€¼ã€‚
  - è´Ÿå‘æµ‹è¯•ï¼ˆæœŸæœ› FAILï¼‰ï¼š
    - ç¼ºå°‘å…­ä¸ª `Session` æ–¹æ³•ï¼ˆ`Prompt`ã€`Cancel`ã€`SetMode`ã€`AdapterName`ã€`SessionID`ã€`Close`ï¼‰ä¸­ä»»æ„ä¸€ä¸ªçš„ struct æ— æ³•ä½œä¸º `agent.Session` ç¼–è¯‘é€šè¿‡ã€‚

- AC-4: `provider.Provider` æŽ¥å£å’Œ `provider/codex.CodexAdapter` ä½œä¸ºæ— çŠ¶æ€è¿žæŽ¥å·¥åŽ‚æ­£ç¡®å·¥ä½œã€‚
  - æ­£å‘æµ‹è¯•ï¼ˆæœŸæœ› PASSï¼‰ï¼š
    - `CodexAdapter.Connect(ctx)` è¿”å›žçš„ `*acp.Conn` å­è¿›ç¨‹å·²åœ¨è¿è¡Œï¼ˆè°ƒç”¨æ–¹æ— éœ€å†æ¬¡è°ƒç”¨ `Start()`ï¼‰ã€‚
    - ä¸¤æ¬¡è°ƒç”¨ `CodexAdapter.Connect(ctx)` äº§ç”Ÿä¸¤ä¸ªç‹¬ç«‹çš„ `*acp.Conn` å®žä¾‹ï¼Œå„è‡ªæœ‰ç‹¬ç«‹å­è¿›ç¨‹ã€‚
    - `Connect` æˆåŠŸåŽè°ƒç”¨ `CodexAdapter.Close()` æ˜¯ no-opï¼ˆä¸ä¼šæ€æ­»å·²è½¬ç§»æ‰€æœ‰æƒçš„å­è¿›ç¨‹ï¼‰ã€‚
  - è´Ÿå‘æµ‹è¯•ï¼ˆæœŸæœ› FAILï¼‰ï¼š
    - æ‰¾ä¸åˆ°äºŒè¿›åˆ¶æ–‡ä»¶æ—¶ï¼Œ`CodexAdapter.Connect(ctx)` è¿”å›ž errorã€‚

- AC-5: `client.Client` ä»¥æ–°è®¾è®¡æ›¿æ¢ `hub.Hub`ï¼›å‘½ä»¤è·¯ç”±å’Œæµå¼å›žå¤ç«¯åˆ°ç«¯æ­£å¸¸å·¥ä½œã€‚
  - æ­£å‘æµ‹è¯•ï¼ˆæœŸæœ› PASSï¼‰ï¼š
    - `Client.HandleMessage` å¤„ç† `/use codex` æ—¶è§¦å‘ `switchAdapter`ï¼Œè°ƒç”¨ `provider.Connect()` å¹¶æ›¿æ¢æ´»è·ƒ sessionã€‚
    - `Client.HandleMessage` å¤„ç† `/use codex --continue` æ—¶ä»¥ `SwitchWithContext` æ¨¡å¼è§¦å‘ `switchAdapter`ã€‚
    - `Client.HandleMessage` å¤„ç† `/cancel` æ—¶è°ƒç”¨ `session.Cancel()`ã€‚
    - `Client.HandleMessage` å¤„ç† `/status` æ—¶è¿”å›žåŒ…å« adapter åç§°å’Œ session ID çš„å­—ç¬¦ä¸²ã€‚
    - `Client.Run(ctx)` åœ¨æ—  IM adapter æ—¶é©±åŠ¨ stdin è¯»å¾ªçŽ¯ï¼Œå¹¶å°†æ¯è¡Œè½¬å‘ä¸º `im.Message`ã€‚
    - `Client.Start(ctx)` ç«‹å³è°ƒç”¨ `provider.Connect()`ï¼ˆéžæƒ°æ€§ï¼‰ï¼Œé¦–æ¡ prompt å‰å­è¿›ç¨‹å·²å°±ç»ªã€‚
    - `Client.Close()` å°†å½“å‰ session ID ä¿å­˜åˆ° storeã€‚
  - è´Ÿå‘æµ‹è¯•ï¼ˆæœŸæœ› FAILï¼‰ï¼š
    - `/use unknown-adapter` è¿”å›žæŒ‡ç¤º adapter æœªæ³¨å†Œçš„é”™è¯¯ä¿¡æ¯ã€‚
    - æ³¨å…¥ `client.Client` çš„ mock `agent.Session` åœ¨è°ƒç”¨ `switchAdapter` æ—¶ä¸å¼•å‘ç±»åž‹æ–­è¨€ panicï¼ˆå› ä¸º `Switch` é€šè¿‡ `c.agent *agent.Agent` è°ƒç”¨ï¼Œè€Œéž `Session` æŽ¥å£ï¼‰ã€‚

- AC-6: `agent.Agent.Switch` å®žçŽ° Â§7.4 ä¸­è§„å®šçš„å¹¶å‘å®‰å…¨çº¦å®šã€‚
  - æ­£å‘æµ‹è¯•ï¼ˆæœŸæœ› PASSï¼‰ï¼š
    - `switchAdapter`ï¼ˆClient ä¾§ï¼‰åœ¨è°ƒç”¨ `c.agent.Switch(...)` å‰å…ˆè°ƒç”¨ `c.session.Cancel()`ï¼Œå†æŽ’å¹² `c.currentPromptCh`ã€‚
    - `Switch` åŽï¼Œæ—§ `*acp.Conn` å·²å…³é—­ï¼ˆæ—§å­è¿›ç¨‹ç»ˆæ­¢ï¼‰ï¼Œ`c.agent` æŒæœ‰æ–° `*acp.Conn`ã€‚
    - `SwitchWithContext` ä¸” `lastReply` éžç©ºæ—¶ï¼Œå°†å…¶ä½œä¸ºå¼•å¯¼ prompt å‘é€è‡³æ–° sessionï¼›æ¶ˆè´¹ goroutine å·²å¯åŠ¨ä¸”ä¸æ³„æ¼ã€‚
    - `SwitchWithContext` ä¸” `lastReply` ä¸ºç©ºæ—¶ï¼Œé™é»˜é™çº§ä¸º `SwitchClean` è¡Œä¸ºï¼ˆä¸å‘é€ promptï¼Œä¸è¿”å›ž errorï¼‰ã€‚
    - `SwitchWithContext` ä¸­å¼•å¯¼ `Prompt` è°ƒç”¨å¤±è´¥æ—¶ï¼Œ`Switch` è¿”å›ž `nil`ï¼ˆè€Œéž errorï¼‰ï¼Œå¹¶è®°å½• warningã€‚
  - è´Ÿå‘æµ‹è¯•ï¼ˆæœŸæœ› FAILï¼‰ï¼š
    - `Agent.Switch` å†…éƒ¨ä¸ç­‰å¾…é£žè¡Œä¸­çš„ `Prompt` å®Œæˆâ€”â€”è¿™æ˜¯è°ƒç”¨æ–¹çš„æ–‡æ¡£åŒ–èŒè´£ï¼Œä»¥é˜²æ­»é”ã€‚

- AC-7: çŠ¶æ€æŒä¹…åŒ–é€æ˜Žè¿ç§»æ—§ JSON keyã€‚
  - æ­£å‘æµ‹è¯•ï¼ˆæœŸæœ› PASSï¼‰ï¼š
    - `client.JSONStore.Load()` è¯»å–å« `"active_agent"` å’Œ `"acp_session_ids"` çš„çŠ¶æ€æ–‡ä»¶æ—¶ï¼Œæ­£ç¡®æ˜ å°„åˆ°åŠ è½½åŽ `State` çš„ `ActiveAdapter` å’Œ `SessionIDs`ã€‚
    - `client.JSONStore.Save()` ä»…å†™å…¥æ–° keyï¼ˆ`"activeAdapter"`ã€`"session_ids"`ï¼‰ï¼Œä¸å†™æ—§ keyã€‚
    - ä½¿ç”¨æ—§æ ¼å¼çŠ¶æ€æ–‡ä»¶ï¼ˆä»…å«æ—§ keyï¼‰å¯åŠ¨çš„è¿›ç¨‹èƒ½æ­£ç¡®æ¢å¤ session ID å’Œæ´»è·ƒ adapterã€‚
  - è´Ÿå‘æµ‹è¯•ï¼ˆæœŸæœ› FAILï¼‰ï¼š
    - ä»…å«æ—§ keyï¼ˆ`active_agent`ã€`acp_session_ids`ï¼‰ã€ä¸å«æ–° key çš„çŠ¶æ€æ–‡ä»¶åœ¨ `Load()` åŽä¸ä¼šå¯¼è‡´ `ActiveAdapter` ä¸ºç©ºã€‚

## è·¯å¾„è¾¹ç•Œ

è·¯å¾„è¾¹ç•Œå®šä¹‰äº†å®žçŽ°è´¨é‡å’Œé€‰æ‹©çš„å¯æŽ¥å—èŒƒå›´ã€‚

### ä¸Šç•Œï¼ˆæœ€å¤§å¯æŽ¥å—èŒƒå›´ï¼‰
å®žçŽ°åŒ…å« Â§12 ä¸­æ‰€æœ‰åŒ…é‡å‘½å/è¿ç§»ã€Â§6 ä¸­æ‰€æœ‰æ–°æŽ¥å£å’Œå…·ä½“ç±»åž‹ã€`agent.Agent` å†…éƒ¨æ‹†åˆ†ä¸ºå¤šæ–‡ä»¶ï¼ˆsession.goã€prompt.goã€callbacks.goã€terminal.goã€permission.goã€update.goï¼‰ã€`agent.Agent` å•å…ƒæµ‹è¯•ï¼ˆä½¿ç”¨ Â§11 ä¸­çš„ mock Conn å­è¿›ç¨‹æ¨¡å¼ï¼‰å’Œ `client.Client` å•å…ƒæµ‹è¯•ï¼ˆä½¿ç”¨ mock `agent.Session`ï¼‰ã€`provider/codex` çš„é›†æˆæµ‹è¯•ï¼ˆ`//go:build integration`ï¼‰ï¼Œä»¥åŠå®Œæ•´çš„å‘åŽå…¼å®¹çŠ¶æ€è¿ç§»ã€‚

### ä¸‹ç•Œï¼ˆæœ€å°å¯æŽ¥å—èŒƒå›´ï¼‰
å®žçŽ°å®Œæˆæ‰€æœ‰æŒ‡å®šçš„åŒ…é‡å‘½å/è¿ç§»ï¼Œ`go build ./...` é€šè¿‡ï¼Œå…¨éƒ¨ 16 ä¸ªè¿ç§»åŽçš„ `acp/conn` å•å…ƒæµ‹è¯•é€šè¿‡ï¼Œstdin å¾ªçŽ¯ç«¯åˆ°ç«¯å¯ç”¨ï¼ˆ`go run ./cmd/wheelmaker/`ï¼‰ã€‚`agent.Agent` å’Œ `client.Client` çš„å•å…ƒæµ‹è¯•å¯æŽ¨è¿Ÿï¼Œå‰ææ˜¯ä»£ç ç»“æž„å…·å¤‡å¯æµ‹è¯•æ€§ï¼ˆSession æŽ¥å£å¯æ³¨å…¥ Clientï¼‰ã€‚

### å…è®¸çš„é€‰æ‹©
> **æ³¨æ„**ï¼šè¿™æ˜¯é«˜åº¦ç¡®å®šæ€§çš„é‡æž„ï¼Œå‡ ä¹Žæ‰€æœ‰ç»“æž„æ€§å†³ç­–å‡ç”±è§„èŒƒå›ºå®šã€‚ä»…å‰©å°‘é‡å®žçŽ°å±‚é¢çš„é€‰æ‹©ã€‚

- å¯ä»¥ï¼šåœ¨ `internal/agent/` å†…éƒ¨ä»¥ä»»æ„æ–¹å¼æ‹†åˆ†æ–‡ä»¶ï¼Œåªè¦å…¬å…± API ä¸Ž Â§6 å®Œå…¨åŒ¹é…ã€‚
- å¯ä»¥ï¼šå•ç‹¬åˆ›å»º `conn_integration_test.go`ï¼Œæˆ–åœ¨åŒä¸€æ–‡ä»¶ä¸­ä½¿ç”¨ `//go:build integration` æ ‡ç­¾ã€‚
- å¯ä»¥ï¼šåœ¨ `agent.Agent` å®žçŽ°æ–‡ä»¶å†…ä½¿ç”¨ä»»æ„å†…éƒ¨è¾…åŠ©å‘½åã€‚
- ä¸å¯ä»¥ï¼šä»»ä½•æœªå°† `internal/provider/` ä½œä¸ºç‹¬ç«‹åŒ…å¹¶å¼•å…¥ `Adapter` æŽ¥å£çš„æž¶æž„ã€‚
- ä¸å¯ä»¥ï¼šä»»ä½•å°† `agent.Agent` ä¿ç•™ä¸ºæŽ¥å£çš„æž¶æž„ï¼ˆå¿…é¡»æ”¹ä¸ºå…·ä½“ structï¼›`Session` æ‰æ˜¯çª„æŽ¥å£ï¼‰ã€‚
- ä¸å¯ä»¥ï¼šåœ¨ `client.Client` ä¸­ä¿ç•™å¯¹ `*codex.Agent` çš„ç±»åž‹æ–­è¨€ä»¥èŽ·å– `SessionID()` æˆ–å…¶ä»–å­—æ®µã€‚
- æ‰€æœ‰å…¬å…±æŽ¥å£æ–¹æ³•ç­¾åã€struct å­—æ®µå’Œ JSON tag å‡æŒ‰ Â§6 å’Œ Â§8 å›ºå®šï¼Œä¸å¯æ›´æ”¹ã€‚

## å¯è¡Œæ€§æç¤ºä¸Žå»ºè®®

> **æ³¨æ„**ï¼šæœ¬èŠ‚ä»…ä¾›å‚è€ƒç†è§£ï¼Œè¿™äº›æ˜¯æ¦‚å¿µæ€§å»ºè®®ï¼Œä¸æ˜¯å¼ºåˆ¶è¦æ±‚ã€‚

### æ¦‚å¿µå®žçŽ°è·¯å¾„

é‡æž„å¯æŒ‰ä¾èµ–é¡ºåºè¿›è¡Œï¼Œç¡®ä¿æ¯ä¸€æ­¥ä»£ç å§‹ç»ˆå¯ç¼–è¯‘ï¼š

```
ä¼ è¾“å±‚è¿ç§»ï¼š
  - å°† internal/acp/ å¤åˆ¶åˆ° internal/agent/acp/
  - å°†ç±»åž‹ Clientâ†’Connï¼Œæ›´æ–°æ‰€æœ‰æ–¹æ³•æŽ¥æ”¶è€…
  - å°† client_test.go é‡å‘½åä¸º conn_test.goï¼Œæ›´æ–°ç±»åž‹å¼•ç”¨
  - ä¿ç•™ internal/acp/ ç›´åˆ°æ‰€æœ‰ä¾èµ–æ–¹æ›´æ–°å®Œæ¯•

Agent æ ¸å¿ƒï¼ˆå¯ä¸Žä¼ è¾“å±‚è¿ç§»å¹¶è¡Œï¼‰ï¼š
  - update.go:     æ–°çš„ UpdateType æžšä¸¾ + æ›´ä¸°å¯Œçš„ Update struct
  - permission.go: PermissionHandler æŽ¥å£ + AutoAllowHandlerï¼ˆå§‹ç»ˆ allow_onceï¼‰
  - terminal.go:   ä»Ž codex/handlers.go æŠ½å– terminalManager
  - callbacks.go:  8 ä¸ª ACP å›žè°ƒå¤„ç†å™¨ï¼ˆfs/terminal/permissionï¼‰
  - session.go:    ensureReadyï¼ˆinitialize â†’ session/load æˆ– session/newï¼‰
  - prompt.go:     Prompt goroutine + sessionUpdateHandler channel æ˜ å°„
  - agent.go:      Session æŽ¥å£ã€SwitchModeã€Agent structã€Newã€Switch

Adapter å±‚ + Clientï¼ˆä¾èµ–ä»¥ä¸Šä¸¤éƒ¨åˆ†å®Œæˆï¼‰ï¼š
  - internal/provider/provider.go:       Adapter æŽ¥å£
  - internal/provider/codex/provider.go: CodexAdapterï¼ˆResolveBinary + Conn.Startï¼‰
  - internal/client/state.go:          å¸¦è¿ç§»é€»è¾‘çš„ Stateï¼ˆè§ Load()ï¼‰
  - internal/client/store.go:          åŒ…å£°æ˜Žä»Ž hub æ”¹ä¸º clientï¼ˆå†…å®¹ä¸å˜ï¼‰
  - internal/client/client.go:         Client struct + æ‰€æœ‰æ–¹æ³•

æŽ¥çº¿ + æ¸…ç†ï¼š
  - cmd/wheelmaker/main.go: ä½¿ç”¨ client.Newã€RegisterProviderã€Startã€Run
  - åˆ é™¤ internal/acp/ã€internal/hub/ã€internal/agent/codex/
  - go build ./... å’Œ go test ./internal/agent/acp/...
```

### ç›¸å…³å‚è€ƒ
- `internal/acp/client.go` â€” å¾…è¿ç§»çš„å®Œæ•´ `Conn` å®žçŽ°ï¼ˆä¸éœ€è¦é‡å†™ï¼Œé€»è¾‘ä¸å˜ï¼‰
- `internal/acp/client_test.go` â€” 16 ä¸ªæµ‹è¯•ï¼Œé‡å‘½å/è¿ç§»è‡³ `conn_test.go`
- `internal/agent/codex/provider.go` â€” `ensureSession`ã€`Prompt`ã€`Cancel`ã€`SetMode` é€»è¾‘ï¼Œæ‹†åˆ†è‡³ `session.go`ã€`prompt.go`ã€`agent.go`
- `internal/agent/codex/handlers.go` â€” æ‰€æœ‰å›žè°ƒå¤„ç†å™¨ + `terminalManager`ï¼Œè¿ç§»è‡³ `callbacks.go` + `terminal.go`
- `internal/hub/hub.go` â€” `HandleMessage`ã€å‘½ä»¤åˆ†å‘ã€å›žå¤æµå¼å¤„ç†ï¼Œè¿ç§»è‡³ `client.go`
- `internal/hub/state.go` â€” `State` structï¼Œéœ€æ›´æ–°å¹¶åŠ å…¥è¿ç§»é€»è¾‘
- `internal/tools/resolve.go` â€” `CodexAdapter.Connect` ç”¨äºŽæŸ¥æ‰¾äºŒè¿›åˆ¶è·¯å¾„

## ä¾èµ–ä¸Žæ‰§è¡Œé¡ºåº

### é‡Œç¨‹ç¢‘

1. **ä¼ è¾“å±‚è¿ç§»**ï¼šè¿ç§»å¹¶é‡å‘½å ACP JSON-RPC ä¼ è¾“å±‚ã€‚
   - æ­¥éª¤ Aï¼šå¤åˆ¶ `internal/acp/` å†…å®¹ï¼Œåˆ›å»º `internal/agent/acp/`ã€‚
   - æ­¥éª¤ Bï¼šå°† `Client` é‡å‘½åä¸º `Conn`ï¼Œæ›´æ–°æ–°åŒ…å†…æ‰€æœ‰å¼•ç”¨ã€‚
   - æ­¥éª¤ Cï¼šæ›´æ–° `conn_test.go` ä½¿ç”¨ `Conn` ç±»åž‹ï¼›éªŒè¯ `go test ./internal/agent/acp/...` å…¨éƒ¨ 16 ä¸ªæµ‹è¯•é€šè¿‡ã€‚
   - æ­¥éª¤ Dï¼šä¿ç•™ `internal/acp/` ç›´åˆ°æ‰€æœ‰ä¾èµ–æ–¹å®Œæˆè¿ç§»ã€‚

2. **Agent æ ¸å¿ƒé‡æž„**ï¼šæž„å»ºæ–°çš„ `agent.Agent` å…·ä½“ struct å’Œ `agent.Session` æŽ¥å£ã€‚
   - æ­¥éª¤ Aï¼šç¼–å†™ `update.go`ï¼ŒåŒ…å«æ›´ä¸°å¯Œçš„ `UpdateType` æžšä¸¾å’Œ `Update` structï¼ˆæ›¿æ¢åŽŸæ¥çš„ç®€å• `Update` string ç±»åž‹ï¼‰ã€‚
   - æ­¥éª¤ Bï¼šç¼–å†™ `permission.go` å’Œ `terminal.go`ï¼ˆä»Ž `codex/handlers.go` æŠ½å–ï¼‰ã€‚
   - æ­¥éª¤ Cï¼šç¼–å†™ `callbacks.go`ï¼ˆå…¨éƒ¨ 8 ä¸ª ACP å›žè°ƒå¤„ç†å™¨ï¼Œä½¿ç”¨ `agent/acp` çš„ `acp.Conn`ï¼‰ã€‚
   - æ­¥éª¤ Dï¼šç¼–å†™ `session.go`ï¼ˆ`ensureReady` ç”Ÿå‘½å‘¨æœŸï¼šinitialize â†’ session/load æˆ– session/newï¼‰ã€‚
   - æ­¥éª¤ Eï¼šç¼–å†™ `prompt.go`ï¼ˆ`Prompt` goroutine å’Œ `sessionUpdateHandler` æ›´æ–°ç±»åž‹æ˜ å°„ï¼‰ã€‚
   - æ­¥éª¤ Fï¼šç¼–å†™ `agent.go`ï¼Œå®šä¹‰ `Session` æŽ¥å£ã€`SwitchMode`ã€`Agent` structã€`New`ã€`Switch` ä»¥åŠå…­ä¸ª `Session` æŽ¥å£æ–¹æ³•å®žçŽ°ã€‚

3. **Adapter å±‚ + Client é‡æž„**ï¼šåˆ›å»º Adapter æŠ½è±¡ï¼Œä»¥ Client æ›¿æ¢ Hubã€‚
   - æ­¥éª¤ Aï¼šåˆ›å»º `internal/provider/provider.go`ï¼ŒåŒ…å« `Adapter` æŽ¥å£ã€‚
   - æ­¥éª¤ Bï¼šåˆ›å»º `internal/provider/codex/provider.go`ï¼ŒåŒ…å« `CodexAdapter`ï¼ˆä½¿ç”¨ `tools.ResolveBinary`ï¼Œåˆ›å»º `acp.Conn`ï¼Œè°ƒç”¨ `Conn.Start()`ï¼Œè¿”å›žå·²å¯åŠ¨çš„ `Conn`ï¼‰ã€‚
   - æ­¥éª¤ Cï¼šåˆ›å»º `internal/client/state.go`ï¼ŒåŒ…å«æ›´æ–°åŽçš„ `State` struct å’Œå‘åŽå…¼å®¹çš„ `Load()`ã€‚
   - æ­¥éª¤ Dï¼šåˆ›å»º `internal/client/store.go`ï¼ˆåŒ…å£°æ˜Žä»Ž `hub` æ”¹ä¸º `client`ï¼Œå†…å®¹ä¸å˜ï¼‰ã€‚
   - æ­¥éª¤ Eï¼šåˆ›å»º `internal/client/client.go`ï¼ŒåŒ…å« `Client` structã€`RegisterProvider`ã€`Start`ã€`Run`ã€`HandleMessage`ã€`switchAdapter`ã€`Close`ã€‚

4. **æ¸…ç†ä¸Žæ”¶å°¾**ï¼šæŽ¥çº¿ `main.go` å¹¶åˆ é™¤æ—§åŒ…ã€‚
   - æ­¥éª¤ Aï¼šæ›´æ–° `cmd/wheelmaker/main.go`ï¼Œä½¿ç”¨ `client.New`ã€`client.RegisterProvider`ã€`c.Start`ã€`c.Run`ã€‚
   - æ­¥éª¤ Bï¼šåˆ é™¤ `internal/acp/` æ•´ä¸ªç›®å½•ã€‚
   - æ­¥éª¤ Cï¼šåˆ é™¤ `internal/hub/` æ•´ä¸ªç›®å½•ã€‚
   - æ­¥éª¤ Dï¼šåˆ é™¤ `internal/agent/codex/` ç›®å½•åŠæ—§çš„ `internal/agent/agent.go` æŽ¥å£æ–‡ä»¶ã€‚
   - æ­¥éª¤ Eï¼šéªŒè¯ `go build ./...`ã€`go vet ./...` å’Œ `go test ./internal/agent/acp/...` å…¨éƒ¨é€šè¿‡ã€‚

é‡Œç¨‹ç¢‘ 1 å’Œ 2 å¯å¹¶è¡ŒæŽ¨è¿›ï¼ˆæ— ç›¸äº’ä¾èµ–ï¼‰ã€‚é‡Œç¨‹ç¢‘ 3 ä¾èµ– 1 å’Œ 2 å‡å®Œæˆã€‚é‡Œç¨‹ç¢‘ 4 ä¾èµ–é‡Œç¨‹ç¢‘ 3ã€‚

## å®žçŽ°å¤‡æ³¨

### ä»£ç é£Žæ ¼è¦æ±‚
- æ‰€æœ‰å®žçŽ°ä»£ç å’Œæ³¨é‡Šå¿…é¡»ä½¿ç”¨è‹±æ–‡ï¼ˆé¡¹ç›®çº¦å®šï¼šä»£ç å’Œæ³¨é‡Šä¸­ä¸å¾—å‡ºçŽ°ä¸­æ–‡ï¼‰ã€‚
- å®žçŽ°ä»£ç å’Œæ³¨é‡Šä¸­ä¸å¾—åŒ…å«è®¡åˆ’ä¸“ç”¨æœ¯è¯­ï¼Œå¦‚ "AC-"ã€"é‡Œç¨‹ç¢‘"ã€"æ­¥éª¤"ã€"é˜¶æ®µ" æˆ–ç±»ä¼¼çš„å·¥ä½œæµæ ‡è®°ã€‚
- è¿™äº›æœ¯è¯­ä»…ç”¨äºŽè®¡åˆ’æ–‡æ¡£ï¼Œä¸åº”å‡ºçŽ°åœ¨æœ€ç»ˆä»£ç åº“ä¸­ã€‚
- ä»£ç ä¸­ä½¿ç”¨æè¿°æ€§ã€é¢†åŸŸç›¸å…³çš„å‘½åï¼ˆå¦‚ `ensureReady`ã€`switchAdapter`ã€`handleCallback`ï¼‰ï¼Œè€Œéžè¿›åº¦æ ‡è®°ã€‚

### è§„èŒƒä¸­çš„å…³é”®å®žçŽ°çº¦æŸ
- `provider.Connect()` å¿…é¡»åœ¨å†…éƒ¨è°ƒç”¨ `Conn.Start()` åŽè¿”å›žï¼Œè°ƒç”¨æ–¹æ— éœ€å†æ¬¡è°ƒç”¨ `Start()`ã€‚
- `Client.Start()` å¿…é¡»ç«‹å³ï¼ˆéžæƒ°æ€§ï¼‰è°ƒç”¨ `provider.Connect()`ï¼Œç¡®ä¿é¦–æ¡æ¶ˆæ¯åˆ°æ¥ä¹‹å‰å­è¿›ç¨‹å·²å°±ç»ªã€‚
- `Agent.Switch()` å¿…é¡»åœ¨äº’æ–¥é”å†…è°ƒç”¨ `killAllTerminals()`ï¼Œåœ¨äº’æ–¥é”å¤–è°ƒç”¨ `oldConn.Close()`ï¼ˆé¿å… `Close` é˜»å¡žæ—¶æŒé”æ­»é”ï¼‰ã€‚
- `SwitchWithContext` å¼•å¯¼ prompt çš„ goroutine å¿…é¡»å®Œæ•´æ¶ˆè´¹è¿”å›žçš„ channelï¼ˆ`for range ch {}`ï¼‰ï¼›çœç•¥æ­¤æ“ä½œä¼šå¯¼è‡´ `Prompt` goroutine æ³„æ¼ã€‚
- `Client.switchAdapter()` å¿…é¡»åœ¨è°ƒç”¨ `c.agent.Switch()` å‰æŽ’å¹² `c.currentPromptCh`ï¼ˆä¸Šæ¬¡ `Prompt` è°ƒç”¨è¿”å›žçš„ channelï¼‰ã€‚
- `Client` åŒæ—¶æŒæœ‰ `session agent.Session` å’Œ `agent *agent.Agent` æŒ‡å‘åŒä¸€å¯¹è±¡ã€‚`Switch` å§‹ç»ˆé€šè¿‡ `c.agent`ï¼ˆå…·ä½“ç±»åž‹ï¼‰è°ƒç”¨ï¼Œè€Œéž `c.session`ï¼ˆæŽ¥å£ï¼‰ï¼Œä»¥é¿å… mock æ³¨å…¥æ—¶çš„ç±»åž‹æ–­è¨€ panicã€‚
- çŠ¶æ€ `Load()` åŒæ—¶å¤„ç†æ—§ keyï¼ˆ`active_agent`ã€`acp_session_ids`ï¼‰å’Œæ–° keyï¼ˆ`activeAdapter`ã€`session_ids`ï¼‰ï¼›`Save()` ä»…å†™å…¥æ–° keyã€‚

--- Original Design Draft Start ---

# WheelMaker æž¶æž„é‡è®¾è®¡è§„èŒƒ

> æ—¥æœŸï¼š2026-03-14
> çŠ¶æ€ï¼šå·²æ‰¹å‡†

## 1. èƒŒæ™¯ä¸Žé—®é¢˜

å½“å‰å®žçŽ°çš„å±‚æ¬¡åˆ’åˆ†å’Œå‘½åå­˜åœ¨ä»¥ä¸‹é—®é¢˜ï¼š

1. `agent.Agent` æ˜¯ interfaceï¼Œ`internal/agent/codex/provider.go` æ˜¯å…¶å®žçŽ°â€”â€”æ–‡ä»¶åå« adapterï¼Œæ¦‚å¿µåå« Agentï¼Œå±‚æ¬¡è¯­ä¹‰æ··ä¹±ã€‚
2. `hub` æ‰¿æ‹…äº†åè°ƒèŒè´£ï¼Œä½† "hub" ä¸æ˜¯ ACP åè®®æœ¯è¯­ï¼›æŒ‰ ACP è§„èŒƒï¼ŒWheelMaker æ•´ä½“æ˜¯ "Client"ã€‚
3. `acp.Client` æ˜¯ä½Žå±‚ JSON-RPC ä¼ è¾“ï¼Œä½†åå­— "Client" å’Œç³»ç»Ÿçº§çš„ "æˆ‘ä»¬æ˜¯ ACP Client" äº§ç”Ÿæ­§ä¹‰ã€‚
4. fs/terminal/permission å›žè°ƒæ··åœ¨ codex åŒ…é‡Œï¼Œåˆ‡æ¢ CLI åŽç«¯æ—¶é€»è¾‘æ— æ³•å¤ç”¨ã€‚
5. æ²¡æœ‰æ˜Žç¡®çš„ Adapter æŠ½è±¡ï¼Œæ— æ³•å¹²å‡€åœ°æ”¯æŒå¤š CLIï¼ˆcodex / claude / æœªæ¥å…¶ä»–ï¼‰ã€‚

## 2. è®¾è®¡ç›®æ ‡

- å‘½åä¸Ž ACP åè®®æ–‡æ¡£å¯¹é½ã€‚
- Agent æ˜¯ ACP åè®®çš„å…·ä½“å°è£…ï¼ŒåŒ…å«æ‰€æœ‰å‡ºç«™è°ƒç”¨å’Œå…¥ç«™å›žè°ƒå¤„ç†ã€‚
- Adapter æ˜¯çº¯ç²¹çš„è¿žæŽ¥ç®¡é“æŠ½è±¡ï¼Œåªè´Ÿè´£"å¯åŠ¨ binaryï¼Œè¿”å›žè¿žæŽ¥"ã€‚
- æ”¯æŒè¿è¡Œæ—¶åˆ‡æ¢ Adapterï¼ˆç”¨æˆ·é€‰æ‹©æ˜¯å¦å»¶ç»­ä¸Šä¸‹æ–‡ï¼‰ã€‚
- Permission ç­–ç•¥å¯æ³¨å…¥ï¼ŒMVP auto-allowï¼ŒPhase 2 è·¯ç”±åˆ° IMã€‚
- Client å±‚é€šè¿‡çª„æŽ¥å£ï¼ˆSessionï¼‰ä¾èµ– Agentï¼Œä¿æŒå¯æµ‹è¯•æ€§ã€‚

## 3. æ•°æ®æµ

```
IM (é£žä¹¦ç­‰)
    â†“ im.Message
client.Client          â† åè°ƒå±‚ï¼šè·¯ç”±å‘½ä»¤ã€ç®¡ç† adapter æ± ã€çŠ¶æ€æŒä¹…åŒ–
    â†“ Session interface
agent.Agent            â† ACP åè®®å°è£…ï¼šä¼šè¯ã€promptã€fs/terminal/permission å›žè°ƒ
    â†“ provider.Connect() â†’ *acp.Conn
provider.Provider        â† è¿žæŽ¥å·¥åŽ‚ï¼šå¯åŠ¨ binaryï¼Œè¿”å›ž Connï¼ˆå¯å¤šæ¬¡è°ƒç”¨ï¼‰
    â†“
agent/acp.Conn         â† ä½Žå±‚ä¼ è¾“ï¼šJSON-RPC 2.0 over stdioï¼Œæ‹¥æœ‰å­è¿›ç¨‹ç”Ÿå‘½å‘¨æœŸ
    â†“
CLI binary             â† codex-acp / claude-acp / ...
```

## 4. å‘½åå˜æ›´

| æ—§å | æ–°å | è¯´æ˜Ž |
|------|------|------|
| `internal/hub` | `internal/client` | WheelMaker æ˜¯ ACP Client |
| `hub.Hub` | `client.Client` | åŒä¸Š |
| `internal/acp/` | `internal/agent/acp/` | acp æ˜¯ agent å±‚å†…éƒ¨ä¼ è¾“ç»†èŠ‚ï¼Œç§»å…¥ agent å­åŒ… |
| `acp.Client` | `acp.Conn` | ä½Žå±‚ä¼ è¾“ï¼Œåå‰¯å…¶å®ž |
| `internal/agent/codex/` | `internal/provider/codex/` | Adapter å½’åˆ°é¡¶å±‚ adapter åŒ… |
| `agent.Agent`ï¼ˆinterfaceï¼‰ | `agent.Session`ï¼ˆçª„æŽ¥å£ï¼‰+ `agent.Agent`ï¼ˆconcrete structï¼‰ | Agent æ˜¯å…·ä½“æ¦‚å¿µï¼›Session æ˜¯ Client ç”¨çš„å¯æµ‹è¯•æŽ¥å£ |
| `hub.State.ACPSessionIDs` | `State.SessionIDs` | å­—æ®µæ”¹åï¼ŒJSON tag åŒæ­¥æ›´æ–°ï¼Œ**state.json éœ€è¿ç§»** |

## 5. åŒ…ç»“æž„

```
internal/
  agent/
    acp/               â† ä½Žå±‚ä¼ è¾“ï¼ˆä»Ž internal/acp/ ç§»å…¥ï¼Œæ˜¯ agent çš„å†…éƒ¨ç»†èŠ‚ï¼‰
      conn.go          â† Conn structï¼ˆåŽŸ client.go renameï¼‰
      conn_test.go     â† åŽŸ client_test.go rename
      types.go         â† ä¸å˜

    agent.go           â† Agent struct + Session interface + å¯¹å¤–æ–¹æ³•
    session.go         â† ACP ç”Ÿå‘½å‘¨æœŸï¼ˆinitialize / session/new / session/loadï¼‰
    prompt.go          â† session/prompt + session/update â†’ Update channel
    callbacks.go       â† fs/* / terminal/* / permission å›žè°ƒ
    terminal.go        â† terminalManager
    permission.go      â† PermissionHandler interface + AutoAllowHandler
    update.go          â† Update / UpdateType ç±»åž‹å®šä¹‰

  adapter/
    provider.go         â† Adapter interfaceï¼ˆè¿žæŽ¥å·¥åŽ‚ï¼ŒConnect è¿”å›ž *acp.Connï¼‰
    codex/
      provider.go       â† CodexAdapter implements Adapter

  client/
    client.go          â† Client structï¼ˆåŽŸ hub.goï¼‰ï¼Œä¾èµ– agent.Session æŽ¥å£
    store.go           â† ä¸å˜
    state.go           â† State å­—æ®µè°ƒæ•´ï¼ˆè§ç¬¬ 8 èŠ‚ï¼‰

  im/                  â† ä¸å˜
  tools/               â† ä¸å˜
```

## 6. æŽ¥å£å®šä¹‰

### 6.1 Adapterï¼ˆè¿žæŽ¥å·¥åŽ‚ï¼‰

```go
// adapter/provider.go
Package provider

import (
    "context"
    "github.com/swm8023/wheelmaker/internal/agent/acp"
)

// Adapter abstracts an ACP-compatible CLI backend.
// It is a stateless connection factory: Connect() spawns a new binary subprocess each time and returns its Conn.
// The subprocess lifecycle is owned by the returned *acp.Conn; provider.Close() is only for cleanup on Connect() failure.
// Calling provider.Close() after a successful Connect() is a no-op (subprocess is managed by the Conn).
// Connect() calls Conn.Start() internally; the subprocess is running when Connect() returns.
type Adapter interface {
    Name() string
    Connect(ctx context.Context) (*acp.Conn, error)
    Close() error
}
```

### 6.2 Sessionï¼ˆClient ä½¿ç”¨çš„çª„æŽ¥å£ï¼Œä¿éšœå¯æµ‹è¯•æ€§ï¼‰

```go
// agent/agent.go
package agent

import "context"

// Session is the narrow interface that client.Client depends on.
// Only the methods Client needs for day-to-day operations are included.
// agent.Agent struct implements this interface; tests can inject mocks.
type Session interface {
    Prompt(ctx context.Context, text string) (<-chan Update, error)
    Cancel() error
    SetMode(ctx context.Context, modeID string) error
    AdapterName() string
    SessionID() string
    Close() error
}
```

### 6.3 PermissionHandler

```go
// agent/permission.go
package agent

// PermissionHandler decides how to respond to CLI permission requests.
// MVP: AutoAllowHandler auto-selects allow_once.
// Phase 2: IMPermissionHandler routes to IM (requires Hub to provide chatID context;
//           interface will need extension or context injection via closure).
type PermissionHandler interface {
    RequestPermission(ctx context.Context,
        params acp.PermissionRequestParams) (acp.PermissionResult, error)
}

// AutoAllowHandler: stateless, always selects allow_once.
type AutoAllowHandler struct{}
```

### 6.4 Update

```go
// agent/update.go
package agent

type UpdateType string

const (
    UpdateText       UpdateType = "text"        // agent_message_chunk
    UpdateThought    UpdateType = "thought"     // agent_thought_chunk
    UpdateToolCall   UpdateType = "tool_call"   // tool_call / tool_call_update
    UpdatePlan       UpdateType = "plan"        // plan
    UpdateModeChange UpdateType = "mode_change" // current_mode_update
    UpdateDone       UpdateType = "done"        // prompt ended, Content = stopReason
    UpdateError      UpdateType = "error"       // error, Err != nil
)

// Update is a streaming update unit sent from Agent to Client.
// Raw is populated only for structured types (tool_call, plan);
// for plain text types (text, thought), Raw is nil.
// For unknown sessionUpdate types, Type uses the raw string value and Raw holds the full params JSON.
type Update struct {
    Type    UpdateType
    Content string // text content (text / thought / plan / stopReason)
    Raw     []byte // raw JSON for structured content (tool_call / plan / unknown types)
    Done    bool
    Err     error
}
```

### 6.5 Agentï¼ˆconcrete structï¼‰

```go
// agent/agent.go
package agent

// Agent is the complete ACP protocol encapsulation.
// It holds an active *acp.Conn, handles outbound ACP calls and inbound CLI callbacks.
// It does not hold an Adapter: the Adapter is used only during Connect by the Client;
// after that the Conn owns the subprocess lifecycle.
type Agent struct {
    name       string                // current adapter name (for identification)
    conn       *acp.Conn             // active ACP connection (owns the subprocess)
    caps       acp.AgentCapabilities // capabilities declared by initialize response
    sessionID  string
    cwd        string
    mcpServers []acp.MCPServer

    permission PermissionHandler     // injectable, defaults to AutoAllowHandler
    terminals  *terminalManager

    lastReply  string   // most recent complete agent reply, used for SwitchWithContext
    mu         sync.Mutex
    ready      bool
}

// New creates an Agent and immediately registers callback handlers on the Conn.
// The caller (Client) is responsible for providing a new Conn during Switch.
func New(name string, conn *acp.Conn, cwd string) *Agent

// --- Session interface implementation ---
func (a *Agent) Prompt(ctx context.Context, text string) (<-chan Update, error)
func (a *Agent) Cancel() error
func (a *Agent) SetMode(ctx context.Context, modeID string) error
func (a *Agent) AdapterName() string
func (a *Agent) SessionID() string
func (a *Agent) Close() error

// --- Extended methods (Client calls directly, not through Session interface) ---
func (a *Agent) SetConfigOption(ctx context.Context, configID, value string) error
func (a *Agent) Switch(ctx context.Context, name string, conn *acp.Conn, mode SwitchMode) error
func (a *Agent) SetPermissionHandler(h PermissionHandler)
```

### 6.6 SwitchMode

```go
// agent/agent.go
type SwitchMode int

const (
    // SwitchClean: discard the current session; new Conn is lazily initialized on next Prompt.
    SwitchClean SwitchMode = iota
    // SwitchWithContext: send the current lastReply as initial context to the new session.
    // If context transfer fails (lastReply is empty or Prompt errors), falls back to SwitchClean behavior with a warning.
    SwitchWithContext
)
```

### 6.7 Clientï¼ˆåŽŸ Hubï¼‰

```go
// client/client.go
package client

// Client is the top-level coordinator of WheelMaker.
// Holds an Adapter pool (stateless factories); holds two references to the Agent:
//   - session agent.Session: narrow interface for Prompt/Cancel/SetMode etc., mockable.
//   - agent  *agent.Agent:   concrete type pointer for Switch and other extended methods, can be nil in tests.
// When switching adapters, Client:
//   1. Calls provider.Connect() to get a new Conn
//   2. Calls c.agent.Switch(newConn, ...) to replace the connection (via concrete type, not Session interface)
//   3. The old Adapter's Close() is a no-op (Conn already owns the subprocess lifecycle)
type Client struct {
    adapters map[string]provider.Provider // "codex" â†’ CodexAdapter (stateless factory)
    session  agent.Session              // current active session (narrow interface, mockable)
    agent    *agent.Agent               // same Agent as concrete type pointer, for Switch and extended methods
    store    Store
    state    *State
    im       im.Adapter                 // nil in CLI/test mode
}

func New(store Store, im im.Adapter) *Client
func (c *Client) RegisterProvider(a provider.Provider)
// Start loads persisted state, creates the initial Agent, and calls Connect() eagerly (non-lazy, subprocess ready immediately).
func (c *Client) Start(ctx context.Context) error
func (c *Client) Run(ctx context.Context) error  // blocking: drives IM event loop or stdin loop
func (c *Client) HandleMessage(msg im.Message)
func (c *Client) Close() error
```

### 6.8 acp.Connï¼ˆåŽŸ acp.Clientï¼‰

```go
// agent/acp/conn.go â€” moved into agent/acp/, renamed, logic unchanged
package acp

// Conn manages the full lifecycle of an ACP-compatible subprocess (stdio JSON-RPC).
// After Connect, Conn exclusively owns the subprocess; Close() closes stdin and waits for process exit.
type Conn struct { /* same fields as original Client */ }

func New(exePath string, env []string) *Conn
func (c *Conn) Start() error
func (c *Conn) Send(ctx context.Context, method string, params any, result any) error
func (c *Conn) Notify(method string, params any) error
func (c *Conn) Subscribe(handler NotificationHandler) (cancel func())
func (c *Conn) OnRequest(handler RequestHandler)
func (c *Conn) Close() error
```

## 7. Agent å†…éƒ¨èŒè´£åˆ†åŒº

### 7.1 Session ç”Ÿå‘½å‘¨æœŸï¼ˆsession.goï¼‰

```
Agent.ensureReady(ctx):
  1. conn.Send("initialize", InitializeParams{...}) â†’ get caps
     InitializeParams declares fs.readTextFile=true, fs.writeTextFile=true, terminal=true
  2. if caps.LoadSession && sessionID != "" â†’ conn.Send("session/load", ...)
  3. else â†’ conn.Send("session/new", {cwd, mcpServers}) â†’ store a.sessionID
  4. conn.OnRequest(a.handleCallback)  â† register all inbound callbacks
```

### 7.2 Prompt æµï¼ˆprompt.goï¼‰

```
Agent.Prompt(ctx, text):
  1. ensureReady(ctx)
  2. cancel := conn.Subscribe(sessionUpdateHandler)  â† filter by this sessionID
  3. goroutine:
       conn.Send("session/prompt", {sessionID, text}, &result)
       â†’ success: send Update{Type: UpdateDone, Content: result.StopReason, Done: true}
       â†’ failure: send Update{Type: UpdateError, Err: err, Done: true}
       defer cancel(); defer close(updates)
  4. sessionUpdateHandler:
       convert session/update subtypes to Update (see table below)
       also append agent_message_chunk text to a.lastReply (finalized at prompt end)
```

`session/update` subtype mapping:

| `sessionUpdate` value | `UpdateType` | `Content` | `Raw` |
|---|---|---|---|
| `agent_message_chunk` | `text` | text | nil |
| `agent_thought_chunk` | `thought` | text | nil |
| `tool_call` / `tool_call_update` | `tool_call` | â€” | full update JSON |
| `plan` | `plan` | â€” | full update JSON |
| `current_mode_update` | `mode_change` | â€” | full update JSON |
| other known types | raw string | â€” | full update JSON |

### 7.3 å…¥ç«™å›žè°ƒï¼ˆcallbacks.goï¼‰

`conn.OnRequest(a.handleCallback)` registered inside `ensureReady`.
Callbacks use `context.Background()` as TODO: future work can pass session-scoped context via `Conn.OnRequest` to support cancellation.

| ACP Method | Handler Logic |
|----------|----------|
| `fs/read_text_file` | `os.ReadFile(params.Path)` |
| `fs/write_text_file` | `os.MkdirAll` + `os.WriteFile` |
| `terminal/create` | `terminalManager.Create(...)` â†’ return `terminalId` |
| `terminal/output` | `terminalManager.Output(terminalID)` |
| `terminal/wait_for_exit` | `terminalManager.WaitForExit(terminalID)` |
| `terminal/kill` | `terminalManager.Kill(terminalID)` |
| `terminal/release` | `terminalManager.Release(terminalID)` |
| `session/request_permission` | `a.permission.RequestPermission(ctx, params)` |

### 7.4 Adapter åˆ‡æ¢ï¼ˆagent.goï¼‰

Switching is coordinated by **Client**; Agent only handles connection replacement:

**Concurrency contract**: Before calling `Switch`, the caller (Client) MUST call `Cancel()` and wait for the current Prompt goroutine to finish (i.e., wait for the channel from the last `Prompt` to close), then call `Switch`.
`Agent` itself does NOT wait for Prompt completion inside `Switch` â€” that is the caller's responsibility to avoid deadlock (Cancel must send a network message; holding a lock while waiting would deadlock).

```
// Client side (client.go):
// Uses c.agent (concrete type) to call Switch, NOT c.session (interface),
// to avoid type assertion panics when a mock is injected.
func (c *Client) switchAdapter(ctx, name, mode):
    // 1. cancel and drain the current prompt (if any)
    c.session.Cancel()
    if c.currentPromptCh != nil:
        for range c.currentPromptCh {}   // wait for Prompt goroutine to exit
        c.currentPromptCh = nil
    // 2. start new binary
    newAdapter = c.adapters[name]
    newConn, err = newAdapter.Connect(ctx)
    if err: reply error
    // 3. replace connection
    c.agent.Switch(ctx, name, newConn, mode)
    // old Conn is closed inside Agent.Switch (via Close)
    // Adapter object stays in c.adapters for future Connect calls (factory semantics)

// Agent side (agent.go):
// Caller guarantees no concurrent Prompt; Switch just needs mu lock for field writes.
func (a *Agent) Switch(ctx, name, newConn, mode):
    a.mu.Lock()
    if mode == SwitchWithContext && a.lastReply != "":
        summary = a.lastReply
    a.killAllTerminals()       // clean up terminals while holding lock
    oldConn = a.conn
    a.conn = newConn
    a.name = name
    a.ready = false
    a.sessionID = ""
    a.lastReply = ""
    a.mu.Unlock()
    oldConn.Close()            // close old Conn outside lock (kills old subprocess), avoids Close blocking with lock held
    if mode == SwitchWithContext && summary != "":
        ch, err = a.Prompt(ctx, "[context] "+summary)
        if err == nil:
            go func() { for range ch {} }()  // must consume channel, otherwise Prompt goroutine leaks
        // Prompt failure does not affect switch success; log warning only
    return nil
```

> **Note**: `Client` must store a `currentPromptCh <-chan Update` field, updated after each `Prompt` call, used in `switchAdapter` for the drain operation.

## 8. çŠ¶æ€æŒä¹…åŒ–

```go
// client/state.go
type AdapterConfig struct {
    ExePath string            `json:"exePath"`
    Env     map[string]string `json:"env"`
}

// State change notes (two breaking changes; Load() must handle both):
//
// 1. Field ACPSessionIDs (json:"acp_session_ids") â†’ SessionIDs (json:"session_ids")
//    Migration: if "session_ids" is empty but "acp_session_ids" is not, copy old value.
//
// 2. Field ActiveAgent (json:"active_agent") â†’ ActiveAdapter (json:"activeAdapter")
//    Migration: if "activeAdapter" is empty but "active_agent" is not, copy old value.
//    (Omitting this causes ActiveAdapter to be empty on startup, silently losing user's adapter selection)
//
// Save writes only new keys; no longer writes old keys.
type State struct {
    ActiveAdapter string                   `json:"activeAdapter"`
    Adapters      map[string]AdapterConfig `json:"adapters"`
    SessionIDs    map[string]string        `json:"session_ids"` // adapter name â†’ ACP sessionId
}

// Load() migration pseudocode:
//   raw := parseJSON(file)
//   if raw["activeAdapter"] == "" && raw["active_agent"] != "":
//       state.ActiveAdapter = raw["active_agent"]
//   if len(raw["session_ids"]) == 0 && len(raw["acp_session_ids"]) > 0:
//       state.SessionIDs = raw["acp_session_ids"]
```

## 9. CLI å‘½ä»¤å¤„ç†

Client.HandleMessage parses `/`-prefixed commands.
`/use` now immediately starts a new subprocess (no longer lazy); old subprocess is synchronously closed.

| Command | Client Behavior |
|------|-------------|
| `/use <name>` | switchAdapter(name, SwitchClean) |
| `/use <name> --continue` | switchAdapter(name, SwitchWithContext) |
| `/cancel` | session.Cancel() |
| `/status` | session.AdapterName() + session.SessionID() |
| other text | session.Prompt() â†’ streaming reply |

> **Behavior change**: Original `/use` only updated `state.ActiveAgent` without starting the subprocess (lazy).
> New design has `/use` immediately Connect, ensuring the first Prompt after switch is faster, and the old process releases resources immediately.

## 10. cmd/wheelmaker/main.go å˜æ›´

```go
func run() error {
    store := client.NewJSONStore(statePath)
    c := client.New(store, nil)
    c.RegisterProvider(codex.NewAdapter(codex.Config{...}))

    ctx, stop := signal.NotifyContext(...)
    defer stop()

    if err := c.Start(ctx); err != nil { return err }
    defer c.Close()

    return c.Run(ctx) // Run blocks: CLI mode drives stdin loop
}
```

`Client.Run(ctx)` drives the stdin read loop when there is no IM adapter (original `main.go` for loop logic moves here);
with an IM adapter it calls `im.provider.Run(ctx)`.

## 11. æµ‹è¯•ç­–ç•¥

| Layer | Approach |
|----|------|
| `agent/acp.Conn` | mock agent (test binary acts as subprocess), existing 16 unit tests (file rename only) |
| `provider/codex` | integration tests, `//go:build integration`, requires real codex-acp binary |
| `agent.Agent` | test binary acts as mock Conn subprocess, tests session lifecycle / prompt / switch / callbacks |
| `client.Client` | inject mock Session (implements agent.Session interface), tests command parsing and routing |

## 12. å˜æ›´èŒƒå›´

**æ–°å»ºï¼š**
- `internal/provider/provider.go`ï¼ˆAdapter interfaceï¼‰
- `internal/provider/codex/provider.go`ï¼ˆCodexAdapterï¼ŒåŽŸ codex adapter + handlers åˆå¹¶ï¼Œæ—  ACP é€»è¾‘ï¼‰
- `internal/agent/acp/conn.go`ï¼ˆåŽŸ `internal/acp/client.go` ç§»å…¥å¹¶é‡å‘½åï¼‰
- `internal/agent/acp/conn_test.go`ï¼ˆåŽŸ `internal/acp/client_test.go` ç§»å…¥å¹¶é‡å‘½åï¼‰
- `internal/agent/acp/types.go`ï¼ˆåŽŸ `internal/acp/types.go` ç§»å…¥ï¼‰
- `internal/agent/session.go`ï¼ˆACP ç”Ÿå‘½å‘¨æœŸï¼‰
- `internal/agent/prompt.go`ï¼ˆprompt æµï¼‰
- `internal/agent/callbacks.go`ï¼ˆå…¥ç«™å›žè°ƒï¼‰
- `internal/agent/terminal.go`ï¼ˆterminalManagerï¼‰
- `internal/agent/permission.go`ï¼ˆPermissionHandler + AutoAllowHandlerï¼‰
- `internal/agent/update.go`ï¼ˆUpdate ç±»åž‹ï¼‰

**é‡å‘½å/é‡å†™ï¼š**
- `internal/hub/` â†’ `internal/client/`ï¼ˆHub â†’ Clientï¼Œæ–°å¢ž Session interface ä¾èµ–ï¼Œæ–°å¢ž Run()ï¼Œæ–°å¢ž currentPromptCh å­—æ®µï¼‰
- `internal/agent/agent.go`ï¼ˆåˆ é™¤ Agent interfaceï¼Œæ”¹ä¸º concrete struct + Session interfaceï¼‰

**åˆ é™¤ï¼š**
- `internal/acp/`ï¼ˆæ•´ä¸ªç›®å½•ï¼Œå†…å®¹å·²ç§»å…¥ `internal/agent/acp/`ï¼‰
- `internal/agent/codex/provider.go`
- `internal/agent/codex/handlers.go`

**ä¸å˜ï¼š**
- `internal/im/im.go`
- `internal/tools/resolve.go`
- `internal/client/store.go`ï¼ˆåŽŸ hub/store.goï¼Œä»…ç§»åŒ…ï¼‰

--- Original Design Draft End ---



