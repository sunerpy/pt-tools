import type { Message, MessagePayloadMap, MessageSender, MessageType } from "./types";

type MessageHandler<K extends MessageType> = (
  payload: MessagePayloadMap[K],
  sender: MessageSender,
) => Promise<unknown> | unknown;

export function createMessage<K extends MessageType>(
  type: K,
  payload: MessagePayloadMap[K],
): Message<MessagePayloadMap[K]> {
  return {
    type,
    payload,
    timestamp: Date.now(),
  };
}

export async function sendToBackground<K extends MessageType>(
  message: Message<MessagePayloadMap[K]>,
): Promise<unknown> {
  return chrome.runtime.sendMessage(message);
}

export async function sendToContent<K extends MessageType>(
  tabId: number,
  message: Message<MessagePayloadMap[K]>,
): Promise<unknown> {
  return chrome.tabs.sendMessage(tabId, message);
}

export async function sendToPopup<K extends MessageType>(
  message: Message<MessagePayloadMap[K]>,
): Promise<unknown> {
  return chrome.runtime.sendMessage(message);
}

export function onMessage<K extends MessageType>(type: K, handler: MessageHandler<K>): () => void {
  const listener = (
    rawMessage: unknown,
    sender: chrome.runtime.MessageSender,
    sendResponse: (response?: unknown) => void,
  ): boolean | void => {
    if (!isTypedMessage(rawMessage) || rawMessage.type !== type) {
      return undefined;
    }

    Promise.resolve(
      handler(rawMessage.payload as MessagePayloadMap[K], {
        tabId: sender.tab?.id,
        frameId: sender.frameId,
        url: sender.url,
      }),
    )
      .then((result) => sendResponse({ ok: true, data: result }))
      .catch((error: unknown) => {
        const message = error instanceof Error ? error.message : "Unknown error";
        sendResponse({ ok: false, error: message });
      });

    return true;
  };

  chrome.runtime.onMessage.addListener(listener);
  return (): void => chrome.runtime.onMessage.removeListener(listener);
}

function isTypedMessage(value: unknown): value is Message<unknown> {
  if (typeof value !== "object" || value === null) {
    return false;
  }

  const candidate = value as Record<string, unknown>;
  return (
    typeof candidate.type === "string" &&
    typeof candidate.timestamp === "number" &&
    "payload" in candidate
  );
}
