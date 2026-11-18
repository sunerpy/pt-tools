/*
 * 语音输入的录音处理器（AudioWorklet）：把麦克风的每一小块单声道采样原样交给页面，页面攒起来以后转成 16 kHz 的 WAV。
 * 用 ?url 引入（构建时放进 /assets/，与页面同源），不写成 Blob，免得以后加了 CSP 加载不了。
 */
/* global AudioWorkletProcessor, registerProcessor */
class PcmTap extends AudioWorkletProcessor {
  process(inputs) {
    const ch = inputs[0] && inputs[0][0];
    if (ch && ch.length) this.port.postMessage(ch.slice(0));
    return true;
  }
}

registerProcessor("pt-pcm-tap", PcmTap);
