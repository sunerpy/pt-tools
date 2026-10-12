# AI assistant

pt-tools has a built-in AI assistant. Once it is connected to a language model you can ask, on the web, in the mobile app or in notification channels (Telegram, QQ), about your site statistics, your downloaders and the torrents pt-tools fetched, and have it search for torrents, push them to a downloader or add subscriptions. Every action that changes something waits for you to confirm it.

The assistant runs inside pt-tools. It calls the same tools as [MCP](mcp.md), executed through the [App API](../reference/app-api.md), so it sees the same data with the same redaction as the app. Any OpenAI-compatible Chat Completions endpoint works, as long as the model supports tool calling (function calling).

## Connecting a model

Open **System → AI assistant settings**:

1. Pick a provider. A preset fills in the endpoint and common models. Below the provider the page says whether it has a free tier and whether it needs a proxy, and **Get an API key** opens the provider's key page.
2. Enter the API key.
3. Pick a model: **Fetch models** reads the provider's model list with the endpoint and key you entered and puts it in the dropdown; you can also type a model name. Pick a model that supports tool calling (function calling).
4. Click **Test connection**: pt-tools sends a very short message and shows the model's reply and the latency.
5. Turn on **Enable AI assistant** and save.

| Provider                    | Endpoint                                                  | Models                                                                     |
| --------------------------- | --------------------------------------------------------- | -------------------------------------------------------------------------- |
| DeepSeek                    | `https://api.deepseek.com`                                | `deepseek-flash`, `deepseek-v4-pro`                                        |
| Qwen (Alibaba Model Studio) | `https://dashscope.aliyuncs.com/compatible-mode/v1`       | `qwen3.7-plus`, `qwen3.8-flash`, `qwen3.8-max`                             |
| Kimi (Moonshot AI)          | `https://api.moonshot.cn/v1`                              | `kimi-k2.6`, `kimi-k3`                                                     |
| SiliconFlow                 | `https://api.siliconflow.cn/v1`                           | Pick one with **Fetch models**                                             |
| Google AI Studio            | `https://generativelanguage.googleapis.com/v1beta/openai` | `gemini-3.8-flash`, `gemini-3.5-flash-lite`, `gemini-2.5-flash` and others |
| Groq                        | `https://api.groq.com/openai/v1`                          | `qwen/qwen3.8-27b`, `openai/gpt-oss-120b`, `openai/gpt-oss-20b`            |
| OpenRouter                  | `https://openrouter.ai/api/v1`                            | Pick one with **Fetch models**                                             |
| Ollama (self-hosted)        | `http://<Ollama host>:11434/v1`                           | A name from `ollama list`; no API key                                      |
| OpenAI                      | `https://api.openai.com/v1`                               | `gpt-6.1-sol` and others                                                   |

The model names are the ones in each provider's documentation in October 2026; the provider's console is authoritative.

- The endpoint is everything before `/chat/completions`, without query parameters.
- The API key is stored encrypted and can be replaced but never read back. If you change the endpoint without entering a new key, saving clears the stored key: a key is only ever sent to the endpoint it was saved with.
- With the proxy left empty pt-tools uses the `HTTP_PROXY` and `HTTPS_PROXY` environment variables; you can also set an `http://`, `https://` or `socks5://` proxy. Fetching models, testing the connection and chatting all go through it. From mainland China, Google AI Studio, Groq and OpenAI usually need a proxy: Google AI Studio answers `User location is not supported` to addresses in mainland China and Hong Kong.
- When pt-tools runs in Docker and Ollama on the host, use the host's address, not `127.0.0.1`.
- Thinking modes (DeepSeek and others) return reasoning content. pt-tools stores it and sends it back to the model in later requests (requests with tools require that), and never shows it in any entry point.

### Using free tiers

With a key of your own you can use these providers' daily free tiers. The tiers change; the provider's pages are authoritative:

| Provider                    | Free tier (October 2026)                                                                                          | Good to know                                                                                        |
| --------------------------- | ----------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------- |
| Google AI Studio            | Gemini Flash and other models are free within per-minute and per-day request limits                               | Content on the free tier may be used by Google to improve its products; needs a proxy               |
| Groq                        | The three models in the table are free, each about 1,000 requests and 200K tokens a day and 8,000 tokens a minute | Few tokens per minute, so questions that need several tool calls wait longer; usually needs a proxy |
| Qwen (Alibaba Model Studio) | A new account gets a free quota for each model; usage beyond it is billed                                         | Turn on the free-quota-only switch for the model in the Model Studio console to avoid charges       |
| SiliconFlow                 | Models marked free in its model square are free, with rate limits                                                 | Pick a model that supports tool calling                                                             |
| OpenRouter                  | Models whose name ends in `:free` are free, with a daily request limit                                            | Pick a model that supports tool calling                                                             |

Small models are often unreliable at tool calling. If answers never call tools, invent tool names or get the arguments wrong, switch to a larger model.

The settings also have three limits:

| Setting                | Default    | Meaning                                                                       |
| ---------------------- | ---------- | ----------------------------------------------------------------------------- |
| Confirmation wait      | 10 minutes | How long an action waits for you, 1 to 60 minutes; after that it does not run |
| Tool rounds per answer | 8          | 1 to 20; at the limit the assistant stops and suggests rephrasing             |
| Keep conversations     | 30 days    | Conversations not updated for this many days are deleted daily; 0 keeps them  |

## Chatting

Open **Overview → AI assistant**: conversations on the left, the chat on the right. You can ask things like:

- How are my sites doing, and which one has the lowest ratio?
- How many torrents are downloading right now, and how much free space is left?
- What did RSS fetch recently?
- Search for free 2160p releases of Dune
- Push the second one to the default downloader
- Subscribe to season 2 of Dune

Answers appear as they are generated. The tools the assistant called are folded under its answer with their arguments and results. You can click **Stop** while it answers; a conversation answers one question at a time.

A torrent download URL you paste is replaced by "site X, torrent Y" before it reaches the model, so the passkey in it never leaves pt-tools. Only URLs of configured sites are recognised; magnet links are not accepted.

## Actions need your confirmation

Before pausing, resuming or deleting a torrent, pushing a torrent to a downloader or adding a subscription, a confirmation card appears in the chat saying what will happen (for example "Push torrent to downloader: site hdsky, torrent 123, downloader 1"). The action runs only when you click **Confirm**; clicking **Deny**, or letting the confirmation wait pass, means it does not run, and the assistant tells you so.

- The model can neither see nor set the confirmation argument; only you can confirm.
- When one answer involves several actions they are confirmed one at a time: the next card appears after the previous one is decided.
- An action counts once: two pages confirming at the same moment, or a confirmation racing the timeout, produce exactly one decision, and the action runs at most once.
- Stopping the answer or restarting pt-tools voids every confirmation still waiting.
- Executed actions are recorded in **ChatOps → Action audit**: the channel is "AI assistant", the command is the tool name, and the user is the conversation's owner (`web:<username>` on the web). Torrents pushed by the assistant show `assistant_push` as their source in the task list.

## In notification channels

With **Notification channels (ChatOps)** turned on in **System → AI assistant settings**, messages from bound Telegram and QQ accounts that are not commands (do not start with `/`) go to the AI assistant; commands stay commands.

- Each binding keeps using one conversation, so earlier turns are part of the context; send `/newchat` to start a new one (after the previous answer finishes).
- The answer is generated in the background and sent when it is ready. A binding gets one answer at a time: a message sent while the previous question is still being answered gets a "still answering" reply.
- Before an action that changes something, the assistant sends "Please confirm: …". Reply "yes" (or "ok", "confirm", 确认) to run it; any other reply cancels the action. If the confirmation wait passes, it does not run and you get a timeout notice.
- Only admin bindings can run actions; for other bindings the assistant does not get those tools. Today every completed binding is an admin, see [ChatOps quick start](chatops-quickstart.md).
- Messages to the assistant are recorded in the action audit with the command `assistant` (without the text); executed actions get their own entry with the channel "AI assistant" and the user `chatops:<binding id>`.
- Replies in notification channels are plain text; long answers are sent as several messages (up to 4; anything beyond that is left out).

## In the app

**More → AI assistant** in the mobile app works like the web page: it opens the latest conversation, shows the answer as it is generated, and shows a confirmation card before an action that changes something.

- Only devices paired with full control get **Confirm** and **Deny**; on a read-only device the assistant does not get the action tools and can only look things up.
- Conversations created in the app are visible only on that device, and web conversations do not appear in the app.
- When the host has not enabled the AI assistant, **More** does not show it.
- For the endpoints, see [App API](../reference/app-api.md#ai-assistant).

## Privacy and security

- The conversation and the tool results (site statistics, torrent titles, downloader state) are sent to the model provider you chose. To keep data on your machine, use a self-hosted Ollama.
- Cookies, passkeys, passwords and torrent download links are never given to the model: tool results come from the App API, which never contains them; pasted download URLs are replaced by site and torrent ID; stored conversations and error messages also mask URL query strings and long tokens.
- Tool results are treated as data: an "instruction" inside a torrent title or a site message is not followed. Images in answers are not shown and links open in a new tab, so an image URL cannot carry data out.
- Settings can only be changed on the web (signed-in sessions only).
- Progress reaches the page over SSE. Behind Nginx this already works because pt-tools sends `X-Accel-Buffering: no`; with other reverse proxies, turn off response buffering for this path.

## Troubleshooting

**Test connection returns 401 or 403**: the API key is wrong or has no access to this model.

**Test connection returns 404**: the endpoint or the model name is wrong. OpenAI's endpoint needs `/v1`.

**Test connection returns 429**: requests are too fast or the free tier is used up. pt-tools retries twice, waiting as long as the provider asks, and then says roughly how long to wait; check your quota in the provider's console, or switch to another model or provider.

**Test connection returns 400 saying User location is not supported**: the provider does not serve your region (Google AI Studio does not serve mainland China or Hong Kong). Set a proxy that can reach it.

**Fetch models returns nothing, or not the model you want**: the list leaves out embedding, speech, image and other models that cannot chat. You can type the name of a model the provider does not list.

**The assistant says a tool does not exist**: the model made up a tool name. Rephrase, or use a model that is better at tool calling.

**The answer says it cannot change anything**: only entry points that are allowed to write give the model the action tools; the web always can.
