import { Window, Events } from "/wails/runtime.js";
import "./windowChrome.css";

export function mountWindowChrome(host, windowName, api) {
  const window = Window.Get(windowName);
  const bar = document.createElement("div");
  bar.className = "window-chrome";
  bar.innerHTML = `<div class="window-brand"><img alt="" class="window-brand-icon"><span>Channel Notes</span></div><div class="window-chrome-drag" aria-hidden="true"></div><div class="window-controls"><button class="window-minimise" title="최소화" aria-label="최소화"><svg viewBox="0 0 16 16"><path d="M3 8h10"/></svg></button><button class="window-maximise" title="최대화" aria-label="최대화"><svg viewBox="0 0 16 16"><rect x="3.5" y="3.5" width="9" height="9" rx=".4"/></svg></button><button class="window-close" title="닫기" aria-label="닫기"><svg viewBox="0 0 16 16"><path d="M4 4l8 8M12 4l-8 8"/></svg></button></div>`;
  host.prepend(bar);
  const icon = bar.querySelector(".window-brand-icon");
  const setIcon = value => { if (value) icon.src = value; };
  api.GetProgramIcon().then(setIcon).catch(() => {});
  Events.On("program:icon", event => setIcon(event.data));
  const maxButton = bar.querySelector(".window-maximise");
  async function updateMaxState() {
    const maximised = await window.IsMaximised();
    maxButton.classList.toggle("maximised", maximised);
    maxButton.title = maximised ? "이전 크기로" : "최대화";
    maxButton.setAttribute("aria-label", maxButton.title);
  }
  bar.querySelector(".window-minimise").onclick = () => window.Minimise();
  maxButton.onclick = async () => { await window.ToggleMaximise(); updateMaxState(); };
  bar.querySelector(".window-close").onclick = () => window.Close();
  bar.querySelector(".window-chrome-drag").ondblclick = async () => { await window.ToggleMaximise(); updateMaxState(); };
  addEventListener("resize", updateMaxState);
  updateMaxState().catch(() => {});
  return { setIcon };
}
