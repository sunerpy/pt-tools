/*
 * 语音输入的录音：麦克风 → AudioWorklet 拿到原始采样 → 停下时重采样成 16 kHz 单声道 16 位 PCM，写成 WAV
 * （Voltip 的本机服务只收 WAV）。不支持 AudioWorklet 的旧浏览器退回 ScriptProcessor。
 * 浏览器只在安全上下文（HTTPS 或 localhost）里给麦克风：不是的话 voiceUnavailable 写明原因。
 */
import workletURL from "./pcmWorklet.js?url";

/** 交给 Voltip 的采样率 */
export const VOICE_SAMPLE_RATE = 16000;
/** 一段最长录多久（秒）：到了自动停下 */
export const VOICE_MAX_SECONDS = 300;

/** 浏览器给不了麦克风时的原因；能用时为空串 */
export function voiceUnavailable(): string {
  if (typeof window === "undefined") return "不在浏览器里";
  if (!window.isSecureContext) {
    return "浏览器只在 HTTPS 或 localhost 下允许用麦克风：用 HTTPS 打开 pt-tools（反向代理或者本机访问）";
  }
  if (!navigator.mediaDevices?.getUserMedia) return "这个浏览器不能录音";
  if (typeof AudioContext === "undefined") return "这个浏览器不能录音";
  return "";
}

/** 把一段采样从 from 重采样到 to（区间取平均；to 不比 from 小时原样返回） */
export function downsample(input: Float32Array, from: number, to: number): Float32Array {
  if (to >= from || input.length === 0) return input;
  const ratio = from / to;
  const out = new Float32Array(Math.floor(input.length / ratio));
  for (let i = 0; i < out.length; i++) {
    const start = Math.floor(i * ratio);
    const end = Math.min(input.length, Math.floor((i + 1) * ratio));
    let sum = 0;
    for (let j = start; j < end; j++) sum += input[j]!;
    out[i] = end > start ? sum / (end - start) : 0;
  }
  return out;
}

/** 把 -1…1 的采样写成 16 位单声道 WAV */
export function encodeWav(samples: Float32Array, sampleRate: number): Blob {
  const buf = new ArrayBuffer(44 + samples.length * 2);
  const v = new DataView(buf);
  const str = (off: number, s: string) => {
    for (let i = 0; i < s.length; i++) v.setUint8(off + i, s.charCodeAt(i));
  };
  str(0, "RIFF");
  v.setUint32(4, 36 + samples.length * 2, true);
  str(8, "WAVE");
  str(12, "fmt ");
  v.setUint32(16, 16, true);
  v.setUint16(20, 1, true);
  v.setUint16(22, 1, true);
  v.setUint32(24, sampleRate, true);
  v.setUint32(28, sampleRate * 2, true);
  v.setUint16(32, 2, true);
  v.setUint16(34, 16, true);
  str(36, "data");
  v.setUint32(40, samples.length * 2, true);
  for (let i = 0; i < samples.length; i++) {
    const s = Math.max(-1, Math.min(1, samples[i]!));
    v.setInt16(44 + i * 2, s < 0 ? s * 0x8000 : s * 0x7fff, true);
  }
  return new Blob([buf], { type: "audio/wav" });
}

/** 一块采样的音量（均方根，0–1），录音时画音量用 */
export function rms(chunk: Float32Array): number {
  if (!chunk.length) return 0;
  let sum = 0;
  for (const x of chunk) sum += x * x;
  return Math.sqrt(sum / chunk.length);
}

/** 一次录音：start 开始，stop 停下并给出 WAV，cancel 丢掉。onLevel 收到每一块的音量。 */
export class VoiceRecorder {
  private stream: MediaStream | null = null;
  private ctx: AudioContext | null = null;
  private node: AudioNode | null = null;
  private source: MediaStreamAudioSourceNode | null = null;
  private chunks: Float32Array[] = [];
  private samples = 0;
  onLevel?: (level: number) => void;

  /** 录了多少秒 */
  get seconds(): number {
    return this.ctx ? this.samples / this.ctx.sampleRate : 0;
  }

  async start(): Promise<void> {
    this.stream = await navigator.mediaDevices.getUserMedia({
      audio: {
        channelCount: 1,
        echoCancellation: true,
        noiseSuppression: true,
        autoGainControl: true,
      },
    });
    const ctx = new AudioContext();
    this.ctx = ctx;
    this.source = ctx.createMediaStreamSource(this.stream);
    const take = (chunk: Float32Array) => {
      this.chunks.push(chunk);
      this.samples += chunk.length;
      this.onLevel?.(rms(chunk));
    };
    if (ctx.audioWorklet) {
      await ctx.audioWorklet.addModule(workletURL);
      const node = new AudioWorkletNode(ctx, "pt-pcm-tap");
      node.port.onmessage = (e: MessageEvent<Float32Array>) => take(e.data);
      this.source.connect(node);
      // 也接到输出上：有的浏览器只处理连到输出的节点。处理器不写输出，接上去是静音，没有回声
      node.connect(ctx.destination);
      this.node = node;
    } else {
      // 旧浏览器：ScriptProcessor（已废弃，只是退路）
      const node = ctx.createScriptProcessor(4096, 1, 1);
      node.onaudioprocess = (e) => take(new Float32Array(e.inputBuffer.getChannelData(0)));
      this.source.connect(node);
      node.connect(ctx.destination);
      this.node = node;
    }
  }

  /** 停下，给出 16 kHz 的 WAV（没录到声音时为 null） */
  async stop(): Promise<Blob | null> {
    const rate = this.ctx?.sampleRate ?? VOICE_SAMPLE_RATE;
    const all = new Float32Array(this.samples);
    let off = 0;
    for (const c of this.chunks) {
      all.set(c, off);
      off += c.length;
    }
    await this.release();
    if (!all.length) return null;
    return encodeWav(downsample(all, rate, VOICE_SAMPLE_RATE), Math.min(rate, VOICE_SAMPLE_RATE));
  }

  /** 丢掉这次录音 */
  async cancel(): Promise<void> {
    await this.release();
  }

  private async release(): Promise<void> {
    this.source?.disconnect();
    this.node?.disconnect();
    for (const t of this.stream?.getTracks() ?? []) t.stop();
    const ctx = this.ctx;
    this.stream = null;
    this.source = null;
    this.node = null;
    this.ctx = null;
    this.chunks = [];
    this.samples = 0;
    if (ctx && ctx.state !== "closed") await ctx.close().catch(() => undefined);
  }
}
