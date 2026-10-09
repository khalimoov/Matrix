import { useEffect, useRef, useState } from "react";

type State = "connecting" | "playing" | "nosignal" | "error";

// LivePlayer shows a camera stream. It uses WebRTC (WHEP) for sub-second
// latency and falls back to HLS when the browser cannot decode the codec
// over WebRTC (for example H.265 in some browsers).
export function LivePlayer({ path, muted = true }: { path: string; muted?: boolean }) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const [state, setState] = useState<State>("connecting");
  const [mode, setMode] = useState<"webrtc" | "hls">("webrtc");

  useEffect(() => {
    const video = videoRef.current!;
    let stopped = false;
    let retry: number | undefined;
    let cleanup = () => {};

    const scheduleRetry = (ms: number) => {
      if (!stopped) retry = window.setTimeout(start, ms);
    };

    async function startWebRTC() {
      const pc = new RTCPeerConnection();
      let session: string | null = null;
      cleanup = () => {
        pc.close();
        if (session) fetch(session, { method: "DELETE", credentials: "same-origin" }).catch(() => {});
      };
      pc.addTransceiver("video", { direction: "recvonly" });
      pc.addTransceiver("audio", { direction: "recvonly" });
      pc.ontrack = (e) => {
        if (video.srcObject !== e.streams[0]) video.srcObject = e.streams[0];
      };
      pc.onconnectionstatechange = () => {
        if (stopped) return;
        if (pc.connectionState === "connected") setState("playing");
        if (pc.connectionState === "failed" || pc.connectionState === "disconnected") {
          setState("connecting");
          cleanup();
          scheduleRetry(2000);
        }
      };
      const offer = await pc.createOffer();
      await pc.setLocalDescription(offer);
      await waitForIce(pc, 1500);
      const res = await fetch(`/media/webrtc/${path}/whep`, {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/sdp" },
        body: pc.localDescription!.sdp,
      });
      if (res.status === 400 || res.status === 406) {
        // The browser cannot receive this codec over WebRTC: switch to HLS.
        const msg = await res.text();
        if (/codec/i.test(msg)) {
          cleanup();
          setMode("hls");
          return;
        }
      }
      if (res.status !== 201) {
        // The camera is not sending video right now (offline or timed out).
        cleanup();
        setState("nosignal");
        scheduleRetry(5000);
        return;
      }
      session = res.headers.get("Location");
      await pc.setRemoteDescription({ type: "answer", sdp: await res.text() });
    }

    async function startHLS() {
      const src = `/media/hls/${path}/index.m3u8`;
      if (video.canPlayType("application/vnd.apple.mpegurl")) {
        video.src = src;
        video.onplaying = () => setState("playing");
        cleanup = () => {
          video.removeAttribute("src");
          video.load();
        };
        return;
      }
      // hls.js is only needed for the fallback, so it is loaded on demand.
      const { default: Hls } = await import("hls.js");
      if (stopped) return;
      if (!Hls.isSupported()) {
        setState("error");
        return;
      }
      const hls = new Hls({ lowLatencyMode: true });
      hls.loadSource(src);
      hls.attachMedia(video);
      hls.on(Hls.Events.FRAG_BUFFERED, () => setState("playing"));
      hls.on(Hls.Events.ERROR, (_e, data) => {
        if (!data.fatal) return;
        setState(data.response?.code === 404 ? "nosignal" : "connecting");
        hls.destroy();
        scheduleRetry(5000);
      });
      cleanup = () => hls.destroy();
    }

    async function start() {
      if (stopped) return;
      setState((s) => (s === "nosignal" ? s : "connecting"));
      try {
        if (mode === "webrtc") await startWebRTC();
        else await startHLS();
      } catch {
        cleanup();
        if (!stopped) {
          setState("error");
          scheduleRetry(5000);
        }
      }
    }

    start();
    return () => {
      stopped = true;
      window.clearTimeout(retry);
      cleanup();
      video.srcObject = null;
    };
  }, [path, mode]);

  return (
    <div className="player">
      <video ref={videoRef} autoPlay playsInline muted={muted} />
      {state !== "playing" && (
        <div className="player-overlay">
          {state === "connecting" && <span className="spinner" />}
          {state === "nosignal" && <span>Нет сигнала</span>}
          {state === "error" && <span>Ошибка подключения</span>}
        </div>
      )}
      {state === "playing" && mode === "hls" && <span className="player-badge">HLS</span>}
    </div>
  );
}

function waitForIce(pc: RTCPeerConnection, timeoutMs: number): Promise<void> {
  if (pc.iceGatheringState === "complete") return Promise.resolve();
  return new Promise((resolve) => {
    const done = () => {
      pc.removeEventListener("icegatheringstatechange", check);
      resolve();
    };
    const check = () => pc.iceGatheringState === "complete" && done();
    pc.addEventListener("icegatheringstatechange", check);
    setTimeout(done, timeoutMs);
  });
}
