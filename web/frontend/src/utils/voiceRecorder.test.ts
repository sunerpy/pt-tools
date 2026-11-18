// @vitest-environment happy-dom
/*
 * 钉子：语音输入的录音工具。重采样按区间取平均、长度按比例缩短；WAV 头是 16 位单声道 PCM、采样率写对；
 * 超出 -1…1 的采样截住不翻转；不是安全上下文时写明麦克风用不了的原因。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { downsample, encodeWav, rms, voiceUnavailable } from "./voiceRecorder";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("voiceRecorder", () => {
  it("重采样：48 kHz 到 16 kHz 每 3 个取平均；目标不比原来低时原样返回", () => {
    const input = Float32Array.from([0, 0.3, 0.6, 1, 1, 1, -1, -1, -0.4, 0.5]);
    const out = downsample(input, 48000, 16000);
    expect(out.length).toBe(3);
    expect([...out].map((x) => Number(x.toFixed(3)))).toEqual([0.3, 1, -0.8]);
    expect(downsample(input, 16000, 16000)).toBe(input);
    expect(downsample(new Float32Array(0), 48000, 16000).length).toBe(0);
  });

  it("WAV：16 位单声道 PCM 的头，采样率与长度写对，超出范围的采样截住", async () => {
    const wav = encodeWav(Float32Array.from([0, 1, -1, 2, -2, 0.5]), 16000);
    expect(wav.type).toBe("audio/wav");
    const buf = await wav.arrayBuffer();
    expect(buf.byteLength).toBe(44 + 6 * 2);
    const v = new DataView(buf);
    const str = (off: number, n: number) => String.fromCharCode(...new Uint8Array(buf, off, n));
    expect(str(0, 4)).toBe("RIFF");
    expect(v.getUint32(4, true)).toBe(36 + 12);
    expect(str(8, 8)).toBe("WAVEfmt ");
    expect(v.getUint16(20, true)).toBe(1); // PCM
    expect(v.getUint16(22, true)).toBe(1); // 单声道
    expect(v.getUint32(24, true)).toBe(16000);
    expect(v.getUint32(28, true)).toBe(32000);
    expect(v.getUint16(34, true)).toBe(16);
    expect(str(36, 4)).toBe("data");
    expect(v.getUint32(40, true)).toBe(12);
    const samples = [0, 1, 2, 3, 4, 5].map((i) => v.getInt16(44 + i * 2, true));
    expect(samples).toEqual([0, 32767, -32768, 32767, -32768, 16383]);
  });

  it("音量：均方根；空块是 0", () => {
    expect(rms(new Float32Array(0))).toBe(0);
    expect(rms(Float32Array.from([0.5, -0.5, 0.5, -0.5]))).toBeCloseTo(0.5);
  });

  it("不是安全上下文（不是 HTTPS 也不是 localhost）：写明原因", () => {
    vi.stubGlobal("isSecureContext", false);
    expect(voiceUnavailable()).toContain("HTTPS");
    vi.stubGlobal("isSecureContext", true);
    vi.stubGlobal("navigator", { mediaDevices: undefined });
    expect(voiceUnavailable()).toBe("这个浏览器不能录音");
  });
});
