/**
 * AudioWorklet processor that captures PCM16 mono audio at 16kHz sample rate.
 * Collects audio samples and posts them to the main thread every ~250ms.
 *
 * Usage:
 *   const audioCtx = new AudioContext({ sampleRate: 16000 })
 *   await audioCtx.audioWorklet.addModule('/audio-processor.js')
 *   const workletNode = new AudioWorkletNode(audioCtx, 'pcm16-processor')
 *   workletNode.port.onmessage = (e) => { ws.send(e.data) }
 */
class PCM16Processor extends AudioWorkletProcessor {
  constructor() {
    super()
    this._buffer = []
    // At 16kHz, 250ms = 4000 samples
    this._flushSize = 4000
    this._stopped = false

    this.port.onmessage = (e) => {
      if (e.data === 'stop') {
        this._stopped = true
        this._flush()
      }
    }
  }

  process(inputs) {
    if (this._stopped) return false

    const input = inputs[0]
    if (!input || !input[0]) return true

    const channel = input[0] // mono channel
    for (let i = 0; i < channel.length; i++) {
      this._buffer.push(channel[i])
    }

    if (this._buffer.length >= this._flushSize) {
      this._flush()
    }

    return true
  }

  _flush() {
    if (this._buffer.length === 0) return

    // Convert Float32 [-1, 1] to Int16 [-32768, 32767]
    const pcm16 = new Int16Array(this._buffer.length)
    for (let i = 0; i < this._buffer.length; i++) {
      const s = Math.max(-1, Math.min(1, this._buffer[i]))
      pcm16[i] = s < 0 ? s * 0x8000 : s * 0x7FFF
    }

    // Post the ArrayBuffer (transferable for zero-copy)
    this.port.postMessage(pcm16.buffer, [pcm16.buffer])
    this._buffer = []
  }
}

registerProcessor('pcm16-processor', PCM16Processor)
