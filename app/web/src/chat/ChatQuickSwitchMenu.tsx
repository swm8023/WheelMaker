import React from 'react';
import type { RegistryChatSession } from '../types/registry';
import type { MobileChatQuickSwitchSection } from './mobileChatQuickSwitch';

export type ChatQuickSwitchMenuPlacement = 'mobile' | 'desktop';

type ChatQuickSwitchMenuProps = {
  sections: MobileChatQuickSwitchSection[];
  placement: ChatQuickSwitchMenuPlacement;
  style: React.CSSProperties;
  isSessionSelected: (projectId: string, session: RegistryChatSession) => boolean;
  renderSessionStateMarker: (session: RegistryChatSession, projectId: string) => React.ReactNode;
  resolveSessionTitle: (session: RegistryChatSession) => string;
  formatSessionAge: (updatedAt: string) => string;
  onSelectSession: (projectId: string, session: RegistryChatSession) => Promise<void> | void;
};

export const ChatQuickSwitchMenu = React.forwardRef<HTMLDivElement, ChatQuickSwitchMenuProps>(
  function ChatQuickSwitchMenu({
    sections,
    placement,
    style,
    isSessionSelected,
    renderSessionStateMarker,
    resolveSessionTitle,
    formatSessionAge,
    onSelectSession,
  }, ref) {
    return (
      <div
        ref={ref}
        className="chat-quick-switch-menu"
        data-placement={placement}
        style={style}
        role="menu"
        aria-label="Recent chats"
        onPointerDown={event => event.stopPropagation()}
      >
        {sections.length === 0 ? (
          <div className="chat-quick-switch-empty">No chats</div>
        ) : (
          sections.map(section => (
            <div key={`chat-quick-switch-project:${section.projectId}`} className="chat-quick-switch-project">
              <div className="chat-quick-switch-project-heading" title={`${section.projectName} - ${section.projectHubLabel}`}>
                <span className="chat-quick-switch-project-name">
                  {section.projectName}
                </span>
                <span className="chat-quick-switch-project-hub">
                  {section.projectHubLabel}
                </span>
              </div>
              <div className="chat-quick-switch-session-list">
                {section.sessions.map(session => {
                  const selected = isSessionSelected(section.projectId, session);
                  const unreadCount = session.unreadCount ?? 0;
                  return (
                    <button
                      key={`chat-quick-switch-session:${section.projectId}:${session.sessionId}`}
                      type="button"
                      className="chat-quick-switch-item"
                      data-selected={selected}
                      role="menuitem"
                      onClick={() => {
                        Promise.resolve(onSelectSession(section.projectId, session)).catch(() => undefined);
                      }}
                    >
                      {renderSessionStateMarker(session, section.projectId)}
                      <span className="chat-quick-switch-title">
                        {resolveSessionTitle(session) || session.sessionId}
                      </span>
                      <span className="chat-quick-switch-time" title={session.updatedAt || ''}>
                        {formatSessionAge(session.updatedAt)}
                      </span>
                      {unreadCount > 0 ? (
                        <span className="chat-quick-switch-unread">{Math.min(99, unreadCount)}</span>
                      ) : null}
                    </button>
                  );
                })}
              </div>
            </div>
          ))
        )}
      </div>
    );
  },
);
