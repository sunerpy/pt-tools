# AI assistant <Badge type="tip" text="1.0 preview" />

pt-tools has a built-in AI assistant. Once it is connected to a language model you can ask, on the web, in the mobile app or in notification channels (Telegram, QQ), about your site statistics, your downloaders and the torrents pt-tools fetched, and have it search for torrents, push them to a downloader, manage subscriptions, recognize torrent titles, organize finished movies and TV shows into your media library, and read or change pt-tools' own configuration. Every action that changes something waits for you to confirm it, and the confirmation card shows what changes from what to what and the possible impact. On the web you can also connect [Voltip](#voice-input) and talk instead of typing.

The assistant runs inside pt-tools. It calls the same tools as [MCP](mcp.md), executed through the [App API](../reference/app-api.md), so it sees the same data with the same redaction as the app. Any OpenAI-compatible Chat Completions endpoint works, as long as the model supports tool calling (function calling).

## Connecting a model

Open **System → AI assistant settings**:

1. Pick a provider. A preset fills in the endpoint and common models. Below the provider the page says whether it has a free tier and whether it needs a proxy, and **Get an API key** opens the provider's key page.
2. Enter the API key.
3. Pick a model: **Fetch models** reads the provider's model list with the endpoint and key you entered and puts it in the dropdown; you can also type a model name. Pick a model that supports tool calling (function calling).
4. Click **Test connection**: pt-tools sends a very short message and shows the model's reply and the latency.
5. Turn on **Enable AI assistant** and save.

| Provider                    | Endpoint                                                  | Models                                                                        |
| --------------------------- | --------------------------------------------------------- | ----------------------------------------------------------------------------- |
| DeepSeek                    | `https://api.deepseek.com`                                | `deepseek-flash`, `deepseek-v4-pro`                                           |
| Qwen (Alibaba Model Studio) | `https://dashscope.aliyuncs.com/compatible-mode/v1`       | `qwen3.7-plus`, `qwen3.8-flash`, `qwen3.8-max`                                |
| Kimi (Moonshot AI)          | `https://api.moonshot.cn/v1`                              | `kimi-k2.6`, `kimi-k3`                                                        |
| SiliconFlow                 | `https://api.siliconflow.cn/v1`                           | Pick one with **Fetch models**                                                |
| Google AI Studio            | `https://generativelanguage.googleapis.com/v1beta/openai` | `gemini-3.5-flash-lite`, `gemini-flash-latest`, `gemini-3.8-flash` and others |
| Groq                        | `https://api.groq.com/openai/v1`                          | `qwen/qwen3.8-27b`, `openai/gpt-oss-120b`, `openai/gpt-oss-20b`               |
| OpenRouter                  | `https://openrouter.ai/api/v1`                            | Pick one with **Fetch models**                                                |
| Ollama (self-hosted)        | `http://<Ollama host>:11434/v1`                           | A name from `ollama list`; no API key                                         |
| OpenAI                      | `https://api.openai.com/v1`                               | `gpt-6.1-sol` and others                                                      |

The model names are the ones in each provider's documentation in October 2026; the provider's console is authoritative.

- The endpoint is everything before `/chat/completions`, without query parameters.
- The API key is stored encrypted and can be replaced but never read back. If you change the endpoint without entering a new key, saving clears the stored key: a key is only ever sent to the endpoint it was saved with.
- With the proxy left empty pt-tools uses the `HTTP_PROXY` and `HTTPS_PROXY` environment variables; you can also set an `http://`, `https://` or `socks5://` proxy. Fetching models, testing the connection and chatting all go through it. From mainland China, Google AI Studio, Groq and OpenAI usually need a proxy: Google AI Studio answers `User location is not supported` to addresses in mainland China and Hong Kong.
- When pt-tools runs in Docker and Ollama on the host, use the host's address, not `127.0.0.1`.
- The model's reasoning (its thinking) is shown on the chat page after redaction. Reasoning returned as `reasoning_content` (DeepSeek, Qwen, Kimi and others) is sent back to the model in later requests (requests with tools require that); reasoning returned as `reasoning` (OpenRouter, Groq, Ollama and others), and `<think>…</think>` at the start of an answer, is shown but not sent back (some providers reject the whole request when they see a field they do not know).

### Using free tiers

With a key of your own you can use these providers' daily free tiers. The tiers change; the provider's pages are authoritative:

| Provider                    | Free tier (October 2026)                                                                                                     | Good to know                                                                                        |
| --------------------------- | ---------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------- |
| Google AI Studio            | Gemini Flash and other models are free within per-minute and per-day request limits; some allow only a few requests a minute | Content on the free tier may be used by Google to improve its products; needs a proxy               |
| Groq                        | The three models in the table are free, each about 1,000 requests and 200K tokens a day and 8,000 tokens a minute            | Few tokens per minute, so questions that need several tool calls wait longer; usually needs a proxy |
| Qwen (Alibaba Model Studio) | A new account gets a free quota for each model; usage beyond it is billed                                                    | Turn on the free-quota-only switch for the model in the Model Studio console to avoid charges       |
| SiliconFlow                 | Models marked free in its model square are free, with rate limits                                                            | Pick a model that supports tool calling                                                             |
| OpenRouter                  | Models whose name ends in `:free` are free, with a daily request limit                                                       | Pick a model that supports tool calling                                                             |

Small models are often unreliable at tool calling. If answers never call tools, invent tool names or get the arguments wrong, switch to a larger model.

The settings also have three limits:

| Setting                | Default    | Meaning                                                                       |
| ---------------------- | ---------- | ----------------------------------------------------------------------------- |
| Confirmation wait      | 10 minutes | How long an action waits for you, 1 to 60 minutes; after that it does not run |
| Tool rounds per answer | 8          | 1 to 20; at the limit the assistant stops and suggests rephrasing             |
| Keep conversations     | 30 days    | Conversations not updated for this many days are deleted daily; 0 keeps them  |

### Context and compaction

The longer a conversation, the more history each request carries. The small ring left of the send button shows how much of the context window this conversation uses: it turns yellow within 15 points of the compaction threshold and red at the threshold. Hover it (tap it on a phone) for the details: about how many tokens the context is, where the window size comes from, when it compacts automatically, and how many input and output tokens the conversation used in total.

| Setting                 | Default  | Meaning                                                                                                                                                               |
| ----------------------- | -------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Context window (tokens) | 0 (auto) | 0 uses the length from the provider's model list, or an estimate from the model name, at most 128K. A smaller value compacts sooner and saves tokens on every request |
| Automatic compaction    | On       | When the context reaches this share of the window, the model first summarizes the earlier conversation before answering                                               |
| Compaction threshold    | 80%      | 50% to 95%                                                                                                                                                            |

- While it compacts, the process says "Compacting the context…"; afterwards a notice in the chat says how many earlier messages became the summary. From then on the model gets the summary plus the messages after it; the page still shows every message.
- Once a conversation has been compacted, it compacts automatically again only after at least 4 more messages (about two exchanges), so a very small window where the system prompt and tool definitions take most of it does not compact on every question.
- When the provider rejects a request because the context is too long, pt-tools compacts once and tries again.
- With automatic compaction off, the oldest messages are dropped when the history gets too long.
- Writing the summary is a model call too and counts toward the conversation's usage. When the provider does not report usage, the context is estimated from the text and the details say "about".

## Chatting

Open **Overview → AI assistant**: conversations on the left (the search box finds them by title; they are grouped into today, yesterday, the last 7 days and earlier; hovering a conversation shows its delete button, which is always shown on phones), the chat on the right with the input box under it. A new chat suggests a few questions; clicking one puts it in the input box. You can ask things like:

- How are my sites doing, and which one has the lowest ratio?
- How many torrents are downloading right now, and how much free space is left?
- What did RSS fetch recently?
- Search for free 2160p releases of Dune
- Push the second one to the default downloader
- Subscribe to season 2 of Dune; which episodes are my subscriptions still missing?
- What is this release: `Dune.Part.Two.2024.2160p.WEB-DL.H265-ABC`
- Which library organizing failed recently? Organize them again

Answers appear as they are generated:

- **Process**: above the answer is a timeline of how it was produced, in order: each piece of thinking (when the model returns its reasoning, it fills in as it is generated) and each tool call (name, arguments and how long it took; open one to see the data it returned), one stretch per round. While the answer is running, the top line says what is happening, for example "Checking site statistics…".
- **Automatic folding**: while the answer is running, the process is expanded; when it is done it folds into one line (for example "Thought · called 3 tools (2 rounds) · took 12 s") that you can click to open. Actions waiting for your confirmation are not folded: their cards sit below the process, above the answer.
- Under the answer are a copy button and the model and reasoning effort used for it.

You can click **Stop** while it answers; a conversation answers one question at a time.

### Choosing the model and reasoning effort

Two drop-downs under the input box apply to each message:

- **Model**: the model from the settings by default. When you open it, pt-tools reads the provider's model list once (cached for 10 minutes); you can also type a model name. Opening a conversation picks the model last used in it.
- **Reasoning effort**: default, no thinking, low, medium or high, sent to the provider as `reasoning_effort` (`none`, `low`, `medium`, `high`; default leaves the parameter out so the model uses its own). Providers differ: when a provider rejects the request because of it, pt-tools retries once without it and adds a notice to the conversation saying the model does not take that effort level.

### Sending images and text files

Click the paper clip under the input box, or paste or drop files into it. A message takes up to 4 attachments, and a message can be just attachments.

| Kind       | Formats                                                                                          | Size        |
| ---------- | ------------------------------------------------------------------------------------------------ | ----------- |
| Images     | PNG, JPEG, WebP, GIF (recognised by content; SVG is not accepted)                                | 5 MB each   |
| Text files | UTF-8 `.txt` `.md` `.log` `.json` `.csv` `.yaml` `.toml` `.ini` `.xml` `.nfo` `.srt` and similar | 200 KB each |

- A text file is redacted first, like what you type (passkeys in URLs, cookies, API keys and long tokens are masked), then put into the message for the model. Text files in earlier messages are included while the history stays within its size limit; ones that do not fit are replaced by a one-line placeholder.
- Images go to the model with that one message only. Later questions rely on what the model already said about them; send the image again if it should look again. When the model cannot read images the provider rejects the request and the error says the model may not support images: pick a model that does, or send without the image.
- Upload starts as soon as you pick a file. Attachments not sent yet are visible only to you and are removed after a day; sent attachments are kept with the conversation and deleted with it. PDFs, archives and torrent files are not accepted.

A torrent download URL you paste is replaced by "site X, torrent Y" before it reaches the model, so the passkey in it never leaves pt-tools. Only URLs of configured sites are recognised; magnet links are not accepted.

## Actions need your confirmation

Before pausing, resuming or deleting a torrent, pushing a torrent to a downloader, adding, pausing or resuming, searching or deleting a subscription, organizing into the library, or changing the configuration, pt-tools first works out a preview from the current state, and a confirmation card appears in the chat:

- One sentence saying what will happen, for example "Pause 'Dune.Part.Two…' in downloader 'qb' (52.3 GB, seeding)".
- Changes: each item before and after, for example "State: seeding → paused" or "种子下载目录（download_dir）: /downloads → /data/pt". Secrets only say they are updated, never their value.
- Impact: what it may lead to, for example that a deletion cannot be undone, how much space it frees, or that RSS tasks reload with the new settings.

The action runs only when you click **Confirm**; clicking **Deny**, or letting the confirmation wait pass, means it does not run, and the assistant tells you so. When the preview cannot read the current state (a downloader is unreachable, say), the impact says so and the action still runs after you confirm; when it certainly cannot run (no such torrent or subscription), the assistant tells you directly and no card appears.

- The model can neither see nor set the confirmation argument; only you can confirm.
- When one answer involves several actions they are confirmed one at a time: the next card appears after the previous one is decided.
- An action counts once: two pages confirming at the same moment, or a confirmation racing the timeout, produce exactly one decision, and the action runs at most once.
- Stopping the answer or restarting pt-tools voids every confirmation still waiting.
- Executed actions are recorded in **ChatOps → Action audit**: the channel is "AI assistant", the command is the tool name, and the user is the conversation's owner (`web:<username>` on the web). Torrents pushed by the assistant show `assistant_push` as their source in the task list.

## Changing the configuration

You can have the assistant read and change pt-tools' configuration, for example:

- Change the global download directory to /data/pt
- Disable downloader qb2; make qb1 the default downloader
- Add an RSS feed for hdsky that pushes to qb1, with the "4K" filter rule
- Turn on automatic sign-in for hdsky; move the sign-in window to 7–9 am
- Send the dead-torrent scan notifications to the Telegram channel
- Create a quality profile that only accepts Chinese subtitles

The assistant reads the current state with `get_config` and changes it with `change_config`, reading the built-in `pt-tools-config` skill first (it lists every kind of configuration, its fields and how to change them). Every change shows a confirmation card first, with each item before and after. The change goes through the same endpoint the web settings use, so validation, encryption and hot reload work as when you change it on the web, and errors from that endpoint (such as "the default downloader cannot be disabled") are reported as they are.

It can change: global settings, downloaders, sites (enabled, credentials, speed limits, brush capacity, H&R seeding requirement, automatic sign-in), RSS feeds, filter rules, notification channels, message templates, automatic sign-in, the daily report, the dead-torrent scan, login reminders, the log level, brush tasks, seed transfer rules, reseeding, CookieCloud, CloakBrowser, media recognition, library organizing, sub-categories, media libraries, path mappings, watch folders, media servers, subscription settings, quality profiles, Douban wish lists, recognition words and overrides.

- Secrets are write-only: passwords, cookies, API keys, passkeys, tokens, RSS URLs, and the tokens and URLs in notification channel settings read as "set" or empty; the assistant can write a new value but never read the old one back. Long strings you paste in the chat are masked before they reach the model, so enter cookies, passkeys and the like on the web.
- Not changeable through the assistant: the login password, API tokens, remote access (devices and pairing), the encryption key, the AI assistant's own settings (provider, endpoint, API key, model, proxy, ChatOps switch, voice input), ChatOps bindings and admins, and the qB-compatible entrance.
- Who can change it: always on the web; in notification channels only admin bindings; in the app it depends on the token or device (paired devices have no configuration permission and cannot read or change it).
- Changes are recorded in the action audit with the command `change_config`; the arguments only record which kind of configuration and which fields changed, not the values.

External MCP clients use the same tools; the token also needs the **Config** scope, see [MCP](mcp.md#changing-the-configuration). Scripts can call the [App API's configuration endpoints](../reference/app-api.md#configuration) directly.

## Voice input

With [Voltip](https://voltip.firlab.app/)'s local service connected, the chat page gets a microphone next to the input box: click it and talk, click again to stop, and pt-tools hands the recording to Voltip, puts the text in the input box, and you check it before sending.

1. Turn on Voltip's local service: in the desktop app, open **Settings → Local service**, turn on **Enable local service**, note the **Endpoint** (`http://127.0.0.1:47840/v1` by default) and click **Copy token**. On a server without a desktop use `voltip-server`; `voltip-server --print-token` prints the token. See Voltip's [local service](https://voltip.firlab.app/recognition/service) docs.
2. In pt-tools, open **System → AI assistant settings**, find **Voice input (Voltip)** and enter Voltip's endpoint and access token.
3. Click **Test voice input**: pt-tools reads Voltip's list of processing modes once and says whether it got through. The **Processing (model)** drop-down then lists Voltip's presets and scenes (`voltip` is the default processing set in Voltip, `voltip:raw` skips AI polishing).
4. Choose the **Recognition language** (Chinese, English or automatic) and save.

- Browsers only give a page the microphone over HTTPS or on localhost. Opened as `http://<LAN address>`, the microphone does not work, and the settings page and the microphone say so; put pt-tools behind an HTTPS reverse proxy, or open it as `http://localhost:<port>` on the machine running it.
- Voltip's local service only accepts connections from the same computer by default. When pt-tools runs in Docker or on another machine, `127.0.0.1` means pt-tools itself: to make Voltip listen elsewhere add `--allow-remote` (`voltip-server --listen 0.0.0.0:47840 --allow-remote`). The connection is then unencrypted, so prefer an SSH tunnel or a TLS proxy; in Docker you can also use host networking.
- A recording is at most 5 minutes; at the limit it stops and is recognized. Press Esc while recording to cancel. After you stop, Voltip needs a few seconds to recognize (and polish) the text.
- The recording goes through pt-tools to the Voltip you configured, not to the language model provider; Voltip recognizes it with its own settings (built-in service, a local model or your provider). The access token is stored encrypted and can be replaced but not read; changing the endpoint without entering a new token clears the stored one on save.
- Voice input is only on the web chat page for now, not in the app or notification channels.

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

- Only devices paired with full control get **Confirm** and **Deny**; on a read-only device the assistant does not get the action tools and can only look things up. Paired devices cannot read or change pt-tools' configuration; for clients using an API token, the assistant can read and change it only when the token has the **Config** scope.
- Conversations created in the app are visible only on that device, and web conversations do not appear in the app.
- When the host has not enabled the AI assistant, **More** does not show it.
- The app cannot send attachments or change the model and reasoning effort yet; it uses the model from the settings.
- For the endpoints, see [App API](../reference/app-api.md#ai-assistant).

## Privacy and security

- The conversation, your attachments and the tool results (site statistics, torrent titles, downloader state, library folders) are sent to the model provider you chose. To keep data on your machine, use a self-hosted Ollama.
- With [AI sign-in](site-login-monitoring.md#captchas-and-quiz-questions-with-ai) turned on, sign-in captcha images and quiz questions are sent to this provider too.
- Cookies, passkeys, passwords and torrent download links are never given to the model: tool results come from the App API, which never contains them; pasted download URLs are replaced by site and torrent ID; stored conversations and error messages also mask URL query strings and long tokens.
- Tool results are treated as data: an "instruction" inside a torrent title or a site message is not followed. Images in answers are not shown and links open in a new tab, so an image URL cannot carry data out.
- The AI assistant's own settings (and the voice input settings) can only be changed on the web (signed-in sessions only); the assistant cannot change them.
- Progress reaches the page over SSE. Behind Nginx this already works because pt-tools sends `X-Accel-Buffering: no`; with other reverse proxies, turn off response buffering for this path.

## Troubleshooting

**Test connection returns 401 or 403**: the API key is wrong or has no access to this model.

**Test connection returns 404**: the endpoint or the model name is wrong. OpenAI's endpoint needs `/v1`.

**Test connection returns 429**: requests are too fast or the free tier is used up. When the provider asks to wait 20 seconds or less, pt-tools waits and tries again, at most twice; when it asks for longer, pt-tools does not retry and says roughly how long to wait. Check your quota in the provider's console, or switch to another model or provider.

**Test connection returns 400 saying User location is not supported**: the provider does not serve your region (Google AI Studio does not serve mainland China or Hong Kong). Set a proxy that can reach it.

**It returns 503 saying This model is currently experiencing high demand**: that model is too busy right now (common for new Google AI Studio models). Try again later, or switch to another model such as `gemini-flash-latest`.

**It returns 404 saying the model is no longer available to new users**: the provider still lists the model, but new keys cannot use it (for example `gemini-2.5-flash`). Pick a newer model.

**Fetch models returns nothing, or not the model you want**: the list leaves out embedding, speech, image and similar models by name, and the models left are not all guaranteed to support tool calling: pick one and click **Test connection**. You can type the name of a model the provider does not list.

**The assistant says a tool does not exist**: the model made up a tool name. Rephrase, or use a model that is better at tool calling.

**The answer says it cannot change anything**: only entry points that are allowed to write give the model the action tools; the web always can.

**A notice says the model does not take that reasoning effort**: the provider does not support `reasoning_effort` for this model, or not that value (some models have no "no thinking"), so it answered with the default. Choose **default** to stop sending it.

**Sending an image returns 400**: most likely the model cannot read images. Pick a model that can (vision or VL models, recent Gemini and GPT models), or send without the image.

**A notice says earlier messages were compacted into a summary**: the context reached the compaction threshold, see [Context and compaction](#context-and-compaction). After that the assistant remembers the summary, and very early details may be gone; repeat them when needed.

**A configuration change says there is nothing to change**: the values are the same as now, or a secret was given back as the placeholder `<已设置>` (meaning keep it).

**A configuration change says a field cannot be changed**: that kind of configuration has no such field, or the field can only be set when adding (such as an RSS feed's site or a notification channel's type). The reply lists the fields that can be changed.

**The microphone is grey and asks for HTTPS**: browsers only give a page the microphone over HTTPS or on localhost, see [Voice input](#voice-input).

**Voice input says it cannot reach Voltip**: Voltip's local service is off, or pt-tools cannot reach that address (by default Voltip only accepts connections from the same computer). Click **Test voice input** in the settings to see why.

**Voice input says Voltip does not accept the access token**: the token was regenerated in Voltip; copy it again into the settings.

**Voice input says Voltip is busy**: too many recognitions at once; try again in a few seconds.
