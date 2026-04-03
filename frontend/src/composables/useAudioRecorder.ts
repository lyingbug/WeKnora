import { ref, type Ref, onUnmounted } from 'vue'

export type RecorderStatus = 'idle' | 'connecting' | 'recording' | 'stopping'

/**
 * WebSocket message types sent by the server.
 */
interface ServerMessage {
  type: 'ready' | 'transcript' | 'error' | 'done'
  text?: string
  is_final?: boolean
  message?: string
}

/**
 * Composable for real-time audio recording with streaming ASR via WebSocket.
 *
 * Uses AudioWorklet to capture PCM16 mono 16kHz audio and sends it over WebSocket
 * to the backend, which forwards it to the configured ASR service.
 *
 * @param kbId - Reactive ref to the knowledge base ID
 */
export function useAudioRecorder(kbId: Ref<string>) {
  const status = ref<RecorderStatus>('idle')
  const duration = ref(0)
  const interimText = ref('')   // gray partial text (being recognized)
  const finalSegments = ref<string[]>([]) // confirmed text segments
  const error = ref('')
  const volumeLevel = ref(0)    // 0-1 for volume indicator

  // Internal state (not reactive)
  let audioContext: AudioContext | null = null
  let workletNode: AudioWorkletNode | null = null
  let sourceNode: MediaStreamAudioSourceNode | null = null
  let analyserNode: AnalyserNode | null = null
  let mediaStream: MediaStream | null = null
  let ws: WebSocket | null = null
  let durationTimer: number | null = null
  let volumeTimer: number | null = null

  /**
   * Computed full confirmed text (all final segments joined).
   */
  const finalText = ref('')

  function updateFinalText() {
    finalText.value = finalSegments.value.join('')
  }

  /**
   * Build WebSocket URL for the ASR stream endpoint.
   */
  function buildWSUrl(): string {
    const token = localStorage.getItem('weknora_token') || ''
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const host = import.meta.env.VITE_IS_DOCKER
      ? window.location.host
      : 'localhost:8080'
    return `${protocol}//${host}/api/v1/knowledge-bases/${kbId.value}/asr/stream?token=${encodeURIComponent(token)}`
  }

  /**
   * Start recording: request mic, connect WebSocket, start AudioWorklet.
   */
  async function startRecording(): Promise<void> {
    if (status.value !== 'idle') return
    error.value = ''
    interimText.value = ''
    finalSegments.value = []
    finalText.value = ''
    status.value = 'connecting'

    try {
      // 1. Request microphone access
      mediaStream = await navigator.mediaDevices.getUserMedia({
        audio: {
          channelCount: 1,
          sampleRate: 16000,
          echoCancellation: true,
          noiseSuppression: true,
        }
      })

      // 2. Set up AudioContext at 16kHz
      audioContext = new AudioContext({ sampleRate: 16000 })
      await audioContext.audioWorklet.addModule('/audio-processor.js')

      sourceNode = audioContext.createMediaStreamSource(mediaStream)
      workletNode = new AudioWorkletNode(audioContext, 'pcm16-processor')

      // Set up analyser for volume visualization
      analyserNode = audioContext.createAnalyser()
      analyserNode.fftSize = 256
      sourceNode.connect(analyserNode)
      analyserNode.connect(workletNode)
      workletNode.connect(audioContext.destination)

      // 3. Connect WebSocket
      await connectWebSocket()

      // 4. Forward audio chunks from WorkletNode to WebSocket
      workletNode.port.onmessage = (e: MessageEvent) => {
        if (ws && ws.readyState === WebSocket.OPEN) {
          ws.send(e.data as ArrayBuffer)
        }
      }

      // 5. Start duration timer
      duration.value = 0
      durationTimer = window.setInterval(() => {
        duration.value++
      }, 1000)

      // 6. Start volume monitoring
      startVolumeMonitor()

      status.value = 'recording'
    } catch (err: any) {
      error.value = err.message || 'Failed to start recording'
      status.value = 'idle'
      cleanup()
      throw err
    }
  }

  /**
   * Connect to the ASR WebSocket and wait for "ready" message.
   */
  function connectWebSocket(): Promise<void> {
    return new Promise((resolve, reject) => {
      const url = buildWSUrl()
      ws = new WebSocket(url)

      const timeout = setTimeout(() => {
        reject(new Error('WebSocket connection timeout'))
        ws?.close()
      }, 10000)

      ws.onopen = () => {
        // Wait for "ready" message
      }

      ws.onmessage = (event: MessageEvent) => {
        try {
          const msg: ServerMessage = JSON.parse(event.data)
          handleServerMessage(msg, resolve, timeout)
        } catch {
          // Ignore non-JSON messages
        }
      }

      ws.onerror = () => {
        clearTimeout(timeout)
        reject(new Error('WebSocket connection failed'))
      }

      ws.onclose = (event: CloseEvent) => {
        clearTimeout(timeout)
        if (status.value === 'connecting') {
          reject(new Error(`WebSocket closed: ${event.reason || event.code}`))
        }
        if (status.value === 'recording') {
          status.value = 'idle'
        }
      }
    })
  }

  let wsReadyResolve: ((value: void) => void) | null = null

  /**
   * Handle messages from the ASR WebSocket server.
   */
  function handleServerMessage(
    msg: ServerMessage,
    resolve?: (value: void) => void,
    timeout?: ReturnType<typeof setTimeout>
  ) {
    switch (msg.type) {
      case 'ready':
        if (timeout) clearTimeout(timeout)
        if (resolve) resolve()
        break

      case 'transcript':
        if (msg.is_final && msg.text) {
          finalSegments.value.push(msg.text)
          updateFinalText()
          interimText.value = ''
        } else if (msg.text) {
          interimText.value = msg.text
        }
        break

      case 'error':
        error.value = msg.message || 'ASR error'
        break

      case 'done':
        // Server finished processing
        if (status.value === 'stopping') {
          status.value = 'idle'
        }
        break
    }
  }

  /**
   * Stop recording and get the complete transcription text.
   */
  async function stopRecording(): Promise<string> {
    if (status.value !== 'recording') return finalText.value

    status.value = 'stopping'

    // Signal the worklet to flush remaining audio
    if (workletNode) {
      workletNode.port.postMessage('stop')
    }

    // Wait a moment for the last audio to be sent, then signal stop
    await new Promise(resolve => setTimeout(resolve, 300))

    // Signal the server to stop
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ type: 'stop' }))
    }

    // Wait for "done" message with timeout
    await new Promise<void>((resolve) => {
      const timeout = setTimeout(() => {
        resolve()
      }, 5000)

      if (ws) {
        const origHandler = ws.onmessage
        ws.onmessage = (event: MessageEvent) => {
          try {
            const msg: ServerMessage = JSON.parse(event.data)
            handleServerMessage(msg)
            if (msg.type === 'done') {
              clearTimeout(timeout)
              resolve()
            }
          } catch {
            // ignore
          }
        }
      } else {
        clearTimeout(timeout)
        resolve()
      }
    })

    cleanup()
    status.value = 'idle'

    // Include any remaining interim text in the final result
    if (interimText.value) {
      finalSegments.value.push(interimText.value)
      updateFinalText()
      interimText.value = ''
    }

    return finalText.value
  }

  /**
   * Cancel recording without saving.
   */
  function cancelRecording() {
    cleanup()
    status.value = 'idle'
    interimText.value = ''
    finalSegments.value = []
    finalText.value = ''
    error.value = ''
    duration.value = 0
  }

  /**
   * Start monitoring audio volume for visualization.
   */
  function startVolumeMonitor() {
    if (!analyserNode) return
    const dataArray = new Uint8Array(analyserNode.frequencyBinCount)

    volumeTimer = window.setInterval(() => {
      if (!analyserNode) return
      analyserNode.getByteFrequencyData(dataArray)
      let sum = 0
      for (let i = 0; i < dataArray.length; i++) {
        sum += dataArray[i]
      }
      volumeLevel.value = sum / (dataArray.length * 255)
    }, 50)
  }

  /**
   * Clean up all audio and WebSocket resources.
   */
  function cleanup() {
    if (durationTimer) {
      clearInterval(durationTimer)
      durationTimer = null
    }
    if (volumeTimer) {
      clearInterval(volumeTimer)
      volumeTimer = null
    }
    if (workletNode) {
      workletNode.disconnect()
      workletNode = null
    }
    if (sourceNode) {
      sourceNode.disconnect()
      sourceNode = null
    }
    if (analyserNode) {
      analyserNode.disconnect()
      analyserNode = null
    }
    if (audioContext) {
      audioContext.close()
      audioContext = null
    }
    if (mediaStream) {
      mediaStream.getTracks().forEach(track => track.stop())
      mediaStream = null
    }
    if (ws) {
      if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) {
        ws.close()
      }
      ws = null
    }
    volumeLevel.value = 0
  }

  /**
   * Format duration in seconds to MM:SS string.
   */
  function formatDuration(seconds: number): string {
    const m = Math.floor(seconds / 60).toString().padStart(2, '0')
    const s = (seconds % 60).toString().padStart(2, '0')
    return `${m}:${s}`
  }

  // Auto-cleanup on component unmount
  onUnmounted(() => {
    cleanup()
  })

  return {
    status,
    duration,
    interimText,
    finalText,
    finalSegments,
    error,
    volumeLevel,
    startRecording,
    stopRecording,
    cancelRecording,
    cleanup,
    formatDuration,
  }
}
