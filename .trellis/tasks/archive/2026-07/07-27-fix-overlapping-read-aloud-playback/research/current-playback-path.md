# Current read-aloud playback path

Date: 2026-07-27

## Runtime path

`MessageItem.handleToggleReadAloud` holds local loading/playing state and awaits
`synthesizeSpeech`. Hosted server mode creates or reuses a Go TTS artifact,
downloads it through the File API, creates a detached `Audio` element backed by
an object URL, and calls `play()` in the message component. Browser speech calls
`window.speechSynthesis.speak` and uses a per-message polling interval.

## Proven failure mechanisms

1. Every rendered message owns an independent audio ref, so message A and
   message B can both play and neither knows the other's state.
2. `isPlaying` becomes true only after `audio.play()` resolves. Repeated clicks
   during provider/download/play latency start concurrent operations and the
   newest local ref overwrites the older audio without disposing it.
3. Unmount cleanup can call `pause()` and revoke the object URL while
   `audio.play()` is pending. Chrome rejects that promise with
   `The play() request was interrupted because the media was removed from the
document`; the generic catch reports it as synthesis failure.
4. An async `synthesizeSpeech` completion after unmount has no mounted owner to
   dispose or control it.
5. The API client already accepts `AbortSignal` for synthesis and File download,
   so cancellation can be propagated without changing backend contracts.

## Recommended boundary

Use one tab-scoped coordinator with an operation generation and abort signal.
It must stop audio and browser speech before replacement, ignore/dispose stale
completions, expose a subscribable active-message phase, and distinguish normal
cancellation from real provider/media errors. Message unmount should stop only
when that message owns the coordinator.
