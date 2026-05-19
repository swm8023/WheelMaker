import {
  extractChatConfirmationReply,
  extractChatOptionReplies,
  splitChatConfirmationReplyText,
  splitChatOptionReplyText,
} from '../web/src/chat/chatOptionReplies';

describe('chat option reply extraction', () => {
  test('extracts contiguous A/B/C line-start choices', () => {
    expect(extractChatOptionReplies([
      'Next step?',
      '',
      'A. Refresh current project',
      'B. Refresh every project',
      'C. Use local cache first',
    ].join('\n'))).toEqual([
      {label: 'A', text: 'Refresh current project'},
      {label: 'B', text: 'Refresh every project'},
      {label: 'C', text: 'Use local cache first'},
    ]);
  });

  test('extracts numeric choices only when a choice context is nearby', () => {
    expect(extractChatOptionReplies([
      '第二个问题：更新动作要支持到什么粒度？',
      '',
      '可选：',
      '1. 只支持单包更新',
      '2. 单包 + 全部更新',
      '3. 只展示版本，更新仍交给部署脚本/命令行',
    ].join('\n'))).toEqual([
      {label: '1', text: '只支持单包更新'},
      {label: '2', text: '单包 + 全部更新'},
      {label: '3', text: '只展示版本，更新仍交给部署脚本/命令行'},
    ]);

    expect(extractChatOptionReplies([
      '当前实现的事实：',
      '1. Session 状态来自 session_summary。',
      '2. 流式更新只更新当前 turn。',
      '3. 虚拟列表会复用节点。',
    ].join('\n'))).toEqual([]);
  });

  test('rejects summaries and plan labels that are not active choices', () => {
    expect(extractChatOptionReplies([
      'Samples:',
      '- `A. Direct send / B. Pick from list / C. Insert only`',
      '',
      '方案 A，推荐：左上按钮变成快捷语句菜单。',
      '方案 B：直接铺开 5 个快捷语句按钮。',
    ].join('\n'))).toEqual([]);
  });

  test('ignores code fences and requires at least A and B', () => {
    expect(extractChatOptionReplies([
      '```text',
      'A. Not a choice',
      'B. Still code',
      '```',
      '',
      'A. Only one visible choice',
    ].join('\n'))).toEqual([]);
  });

  test('splits the latest choice block so the original option lines can be replaced inline', () => {
    expect(splitChatOptionReplyText([
      'Pick one:',
      '',
      'A. Apply the small change',
      'B. Keep the existing behavior',
      '',
      'Only reply with the letter.',
    ].join('\n'))).toEqual([
      {type: 'markdown', text: 'Pick one:\n\n'},
      {type: 'option', reply: {label: 'A', text: 'Apply the small change'}},
      {type: 'option', reply: {label: 'B', text: 'Keep the existing behavior'}},
      {type: 'markdown', text: '\n\nOnly reply with the letter.'},
    ]);
  });

  test('keeps explanatory paragraphs as markdown between selectable option headings', () => {
    const text = [
      '第一个问题：**“消息已发送成功”的判定边界应该是哪一个？**',
      '',
      'A. `session.send` 返回 `ok=true` 就算发送成功。  ',
      '推荐度低。它只能说明请求处理过，不一定说明 UI 已收到并对上了服务端 turn。',
      '',
      'B. 服务端发布并被前端收到对应的 `prompt_request` turn 后，才算发送成功。  ',
      '**推荐。** 这能区分“本地已点发送”“服务端已接收/落库”“AI 正在回复”。',
      '',
      'C. 收到第一段 AI 回复后才算成功。  ',
      '太晚。模型排队或长时间思考时会误判为未发送。',
      '',
      '我的推荐是 **B**。',
    ].join('\n');

    expect(extractChatOptionReplies(text)).toEqual([
      {label: 'A', text: '`session.send` 返回 `ok=true` 就算发送成功。'},
      {label: 'B', text: '服务端发布并被前端收到对应的 `prompt_request` turn 后，才算发送成功。'},
      {label: 'C', text: '收到第一段 AI 回复后才算成功。'},
    ]);
    expect(splitChatOptionReplyText(text)).toEqual([
      {type: 'markdown', text: '第一个问题：**“消息已发送成功”的判定边界应该是哪一个？**\n\n'},
      {type: 'option', reply: {label: 'A', text: '`session.send` 返回 `ok=true` 就算发送成功。'}},
      {type: 'markdown', text: '\n推荐度低。它只能说明请求处理过，不一定说明 UI 已收到并对上了服务端 turn。\n\n'},
      {
        type: 'option',
        reply: {
          label: 'B',
          text: '服务端发布并被前端收到对应的 `prompt_request` turn 后，才算发送成功。',
        },
      },
      {type: 'markdown', text: '\n**推荐。** 这能区分“本地已点发送”“服务端已接收/落库”“AI 正在回复”。\n\n'},
      {type: 'option', reply: {label: 'C', text: '收到第一段 AI 回复后才算成功。'}},
      {type: 'markdown', text: '\n太晚。模型排队或长时间思考时会误判为未发送。\n\n我的推荐是 **B**。'},
    ]);
  });

  test('splits numeric choices while preserving explanatory paragraphs', () => {
    const text = [
      '第四个问题：白名单第一版具体放哪些包？',
      '',
      '可选：',
      '1. 第一版只放上面 5 个。  ',
      '最小可用，避免维护面扩大。',
      '',
      '2. 把当前支持的所有 provider 包都放进去。',
      '',
      '我的推荐是 **1**。',
    ].join('\n');

    expect(splitChatOptionReplyText(text)).toEqual([
      {type: 'markdown', text: '第四个问题：白名单第一版具体放哪些包？\n\n可选：\n'},
      {type: 'option', reply: {label: '1', text: '第一版只放上面 5 个。'}},
      {type: 'markdown', text: '\n最小可用，避免维护面扩大。\n\n'},
      {type: 'option', reply: {label: '2', text: '把当前支持的所有 provider 包都放进去。'}},
      {type: 'markdown', text: '\n\n我的推荐是 **1**。'},
    ]);
  });

  test('splits only the latest valid choice block in a message', () => {
    expect(splitChatOptionReplyText([
      'Previous notes:',
      'A. Old option',
      'B. Old alternative',
      '',
      'Current choice:',
      'A. Send A',
      'B. Send B',
    ].join('\n'))).toEqual([
      {type: 'markdown', text: 'Previous notes:\nA. Old option\nB. Old alternative\n\nCurrent choice:\n'},
      {type: 'option', reply: {label: 'A', text: 'Send A'}},
      {type: 'option', reply: {label: 'B', text: 'Send B'}},
    ]);
  });

  test('extracts trailing Chinese confirmation replies with mapped reply text', () => {
    expect(extractChatConfirmationReply('我的推荐是 B。你认这个边界吗？')).toEqual({
      sentence: '你认这个边界吗？',
      replyText: '确认',
    });
    expect(extractChatConfirmationReply('确认这个修正版？')).toEqual({
      sentence: '确认这个修正版？',
      replyText: '确认',
    });
    expect(extractChatConfirmationReply('你同意这个定义吗？')).toEqual({
      sentence: '你同意这个定义吗？',
      replyText: '同意',
    });
    expect(extractChatConfirmationReply('你接受这个例外吗？还是你要更强规则？')).toEqual({
      sentence: '你接受这个例外吗？',
      replyText: '接受',
    });
    expect(extractChatConfirmationReply('版本状态的来源只看“全局 npm 安装”可以吗？')).toEqual({
      sentence: '版本状态的来源只看“全局 npm 安装”可以吗？',
      replyText: '确认',
    });
    expect(extractChatConfirmationReply('点击 Install/Update 前要不要二次确认？')).toEqual({
      sentence: '点击 Install/Update 前要不要二次确认？',
      replyText: '确认',
    });
    expect(extractChatConfirmationReply('read response 的 turns 是否必须是 raw 连续的？')).toEqual({
      sentence: 'read response 的 turns 是否必须是 raw 连续的？',
      replyText: '确认',
    });
  });

  test('does not extract confirmation replies from option prompts, code fences, or English text', () => {
    expect(extractChatConfirmationReply(['A. 确认', 'B. 接受'].join('\n'))).toBeNull();
    expect(extractChatConfirmationReply(['```text', '确认这个修正版？', '```'].join('\n'))).toBeNull();
    expect(extractChatConfirmationReply('Does this look right?')).toBeNull();
  });

  test('splits the confirmation sentence while preserving surrounding markdown', () => {
    expect(splitChatConfirmationReplyText('前文 **说明**。你同意这个定义吗？')).toEqual([
      {type: 'markdown', text: '前文 **说明**。'},
      {type: 'confirmation', reply: {sentence: '你同意这个定义吗？', replyText: '同意'}},
    ]);
  });

  test('consumes paired markdown bold markers around confirmation replies', () => {
    const text = '前文 **你同意这个定义吗？** 后文';

    expect(extractChatConfirmationReply(text)).toEqual({
      sentence: '你同意这个定义吗？',
      replyText: '同意',
    });
    expect(splitChatConfirmationReplyText(text)).toEqual([
      {type: 'markdown', text: '前文 '},
      {type: 'confirmation', reply: {sentence: '你同意这个定义吗？', replyText: '同意'}},
      {type: 'markdown', text: ' 后文'},
    ]);
  });
});
