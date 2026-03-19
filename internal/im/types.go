package im

import "context"

// IMUpdate is a semantic outbound update emitted by client.
// UpdateType usually comes from ACP update types.
type IMUpdate struct {
	ChatID      string
	SessionID   string
	UpdateType  string
	Text        string
	Raw         []byte
	Correlation string
	ReplyTo     string
}

// DecisionKind identifies the decision use case.
type DecisionKind string

const (
	DecisionPermission DecisionKind = "permission"
	DecisionConfirm    DecisionKind = "confirm"
	DecisionSingle     DecisionKind = "single"
	DecisionInput      DecisionKind = "input"
)

// DecisionOption is one selectable choice in a decision prompt.
type DecisionOption struct {
	ID    string
	Label string
	Value string
}

// DecisionRequest asks IM layer to collect one user decision.
type DecisionRequest struct {
	Kind      DecisionKind
	ChatID    string
	MessageID string
	Title     string
	Body      string
	Options   []DecisionOption
	Meta      map[string]string
	Hint      map[string]string
}

// DecisionResult is the normalized decision output.
type DecisionResult struct {
	Outcome  string // selected/cancelled/timeout/invalid
	OptionID string
	Value    string
	ActorID  string
	Source   string // text_reply/card_action/default_policy
}

// UpdateEmitter can render semantic updates.
type UpdateEmitter interface {
	Emit(ctx context.Context, u IMUpdate) error
}

// DecisionRequester can ask IM layer for a structured decision.
type DecisionRequester interface {
	RequestDecision(ctx context.Context, req DecisionRequest) (DecisionResult, error)
}

// OptionSender renders a list of options for user selection.
// How to present options (card/buttons/text) is adapter-specific.
type OptionSender interface {
	SendOptions(chatID, title, body string, options []DecisionOption, meta map[string]string) error
}

// CardActionEvent is a normalized interactive-card callback.
type CardActionEvent struct {
	ChatID    string
	MessageID string
	UserID    string
	Tag       string
	Option    string
	Value     map[string]string
}

// CardActionSubscriber can receive card action events.
type CardActionSubscriber interface {
	OnCardAction(func(CardActionEvent))
}

// HelpOption describes one interactive /help choice.
type HelpOption struct {
	Label   string
	Command string
	Value   string
}

// HelpModel is the runtime help payload resolved from client.
type HelpModel struct {
	Title   string
	Body    string
	Options []HelpOption
}

// HelpResolverSetter allows client to inject realtime help resolver into IM forwarder.
type HelpResolverSetter interface {
	SetHelpResolver(func(ctx context.Context, chatID string) (HelpModel, error))
}
