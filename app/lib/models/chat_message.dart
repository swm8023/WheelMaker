/// Message types rendered in the chat list.
enum MessageType { user, agent, system, debug, options }

/// A single selectable option inside an options-type message.
class OptionItem {
  final String id;
  final String label;
  const OptionItem({required this.id, required this.label});
}

/// A single chat message (user, agent reply, system notice, or a decision prompt).
class ChatMessage {
  final MessageType type;
  final String text;
  final String? title;
  final List<OptionItem> options;

  /// Called when the user taps one of the option buttons.
  final void Function(String optionId)? onSelected;

  final DateTime timestamp;

  ChatMessage._({
    required this.type,
    required this.text,
    this.title,
    this.options = const [],
    this.onSelected,
    DateTime? ts,
  }) : timestamp = ts ?? DateTime.now();

  factory ChatMessage.user(String text) =>
      ChatMessage._(type: MessageType.user, text: text);

  factory ChatMessage.agent(String text) =>
      ChatMessage._(type: MessageType.agent, text: text);

  factory ChatMessage.system(String text) =>
      ChatMessage._(type: MessageType.system, text: text);

  factory ChatMessage.debug(String text) =>
      ChatMessage._(type: MessageType.debug, text: text);

  factory ChatMessage.options({
    required String title,
    required String body,
    required List<OptionItem> options,
    required void Function(String) onSelected,
  }) =>
      ChatMessage._(
        type: MessageType.options,
        text: body,
        title: title,
        options: options,
        onSelected: onSelected,
      );
}
