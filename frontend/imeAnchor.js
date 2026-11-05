// Windows/WebView2 입력기(IME) 커서 위치 보정.
//
// WebView2는 다음 상황에서 IME에 현재 캐럿 위치를 다시 알려주지 않아, 한글
// 조합 중인 글자가 화면 왼쪽 위(0,0)의 별도 조합 창에 표시되는 경우가 있습니다.
//   - 창을 다른 앱이나 다른 Channel Notes 창으로 전환했다가 돌아올 때
//   - 창 크기를 바꿨을 때
//   - 제목 입력창 ↔ 본문 편집기처럼 편집 가능한 요소 사이에서 포커스가
//     옮겨갈 때
//   - 포커스된 편집기의 내용을 통째로 다시 그릴 때
//
// 편집 요소를 한 번 blur()한 뒤 잠시 후 다시 focus()하면 렌더러가 캐럿
// 사각형을 IME에 다시 보내 위치가 복구됩니다.
//
// blur()와 focus() 사이의 지연은 반드시 필요합니다. 같은 태스크에서 붙여서
// 호출하거나 setTimeout(0)으로 호출하면 렌더러가 두 이벤트를 합쳐 버려
// 효과가 없습니다. (MicrosoftEdge/WebView2Feedback#5675, tauri-apps/wry#1908)

const FOCUS_DELAY_MS = 60;
const WINDOW_FOCUS_SETTLE_MS = 80;
const RESIZE_SETTLE_MS = 120;
const POINTER_WAIT_FALLBACK_MS = 500;

const TEXT_INPUT_TYPES = new Set([
  "",
  "text",
  "search",
  "url",
  "tel",
  "email",
  "password",
]);

function isTextEditable(el) {
  if (!el || el.nodeType !== Node.ELEMENT_NODE || !el.isConnected) return false;
  if (el.isContentEditable) return true;
  if (el.tagName === "TEXTAREA") return true;
  if (el.tagName === "INPUT") {
    return TEXT_INPUT_TYPES.has((el.getAttribute("type") || "").toLowerCase());
  }
  return false;
}

function saveSelection(el) {
  if (el.isContentEditable) {
    const selection = getSelection();
    if (!selection?.rangeCount) return null;
    const range = selection.getRangeAt(0);
    if (!el.contains(range.commonAncestorContainer)) return null;
    return { range: range.cloneRange() };
  }
  if (typeof el.selectionStart !== "number") return null;
  return {
    start: el.selectionStart,
    end: el.selectionEnd,
    direction: el.selectionDirection,
  };
}

function restoreSelection(el, saved) {
  if (!saved) return;
  if (saved.range) {
    if (!saved.range.startContainer.isConnected) return;
    const selection = getSelection();
    selection.removeAllRanges();
    selection.addRange(saved.range);
    return;
  }
  try {
    el.setSelectionRange(saved.start, saved.end, saved.direction || "none");
  } catch {
    // 일부 input 타입은 selection API를 지원하지 않습니다.
  }
}

function saveScroll(el) {
  const positions = [];
  for (let node = el; node; node = node.parentElement) {
    if (node.scrollTop || node.scrollLeft) {
      positions.push([node, node.scrollTop, node.scrollLeft]);
    }
  }
  positions.push([null, window.scrollY, window.scrollX]);
  return positions;
}

function restoreScroll(positions) {
  for (const [node, top, left] of positions) {
    if (node) {
      node.scrollTop = top;
      node.scrollLeft = left;
    } else if (window.scrollY !== top || window.scrollX !== left) {
      window.scrollTo(left, top);
    }
  }
}

// installImeAnchorGuard()가 설치되면 프로그램에서도 강제로 보정을 요청할 수
// 있도록 바깥에 노출합니다. (예: 편집기 내용을 innerHTML로 다시 그린 직후)
let requestReanchor = () => {};

/**
 * 포커스된 편집기의 IME 커서 위치를 다음 태스크에서 다시 고정합니다.
 * 에디터 내용을 프로그램적으로 다시 그린 직후 호출하세요.
 */
export function requestImeAnchor() {
  requestReanchor();
}

/**
 * Windows/WebView2에서 IME 위치가 어긋날 수 있는 상황을 감시하고 자동으로
 * 보정합니다. 다른 플랫폼에서는 아무 동작도 하지 않습니다.
 * @returns {() => void} 정리 함수
 */
export function installImeAnchorGuard() {
  if (!/Windows NT/i.test(navigator.userAgent)) return () => {};

  let settleTimer = 0;
  let focusTimer = 0;
  let composing = false;
  let pending = false;
  let pointerDown = false;
  let waitingForPointerUp = false;
  let restoring = false;
  let focusTarget = null;
  let stopInterruptWatch = null;
  let settleForce = false;
  let lastAnchorAt = 0;
  let lastAnchorTarget = null;

  function cancelFocusTimer() {
    if (focusTimer) {
      clearTimeout(focusTimer);
      focusTimer = 0;
    }
    stopInterruptWatch?.();
    stopInterruptWatch = null;
    focusTarget = null;
  }

  function reanchor(force = false) {
    const target = document.activeElement;
    if (!isTextEditable(target) || document.visibilityState === "hidden") return;
    if (composing) {
      pending = true;
      return;
    }

    // 창 활성화와 focusin이 연달아 발생하면 같은 요소를 두 번 보정하게
    // 되므로, 방금 같은 요소를 보정했다면 한 번은 건너뜁니다. 렌더링이나
    // 조합 종료처럼 반드시 다시 맞춰야 하는 경우에는 force로 무시합니다.
    if (!force && target === lastAnchorTarget && Date.now() - lastAnchorAt < 250) return;

    // 이미 보정 중인데 다른 편집 요소로 포커스가 옮겨졌다면 그 요소로
    // 보정을 다시 시작합니다.
    if (focusTimer) {
      if (target === focusTarget) return;
      cancelFocusTimer();
    }

    // blur() 전에 선택 영역과 스크롤 위치를 기억해 둡니다.
    const selection = saveSelection(target);
    const scroll = saveScroll(target);

    // 지연 시간 동안 사용자가 클릭했다면 그 결과를 덮어쓰지 않습니다.
    let interrupted = false;
    const interrupt = () => {
      interrupted = true;
    };
    window.addEventListener("pointerdown", interrupt, true);
    stopInterruptWatch = () => {
      window.removeEventListener("pointerdown", interrupt, true);
    };

    focusTarget = target;
    focusTimer = setTimeout(() => {
      focusTimer = 0;
      stopInterruptWatch?.();
      stopInterruptWatch = null;
      focusTarget = null;
      if (interrupted) return;
      if (!target.isConnected || composing) return;

      // 기다리는 동안 사용자가 다른 곳으로 포커스를 옮겼다면 가로채지 않습니다.
      const active = document.activeElement;
      if (active && active !== document.body && active !== target) return;

      restoring = true;
      try {
        target.focus({ preventScroll: true });
        restoreSelection(target, selection);
        restoreScroll(scroll);
      } finally {
        restoring = false;
      }
      lastAnchorAt = Date.now();
      lastAnchorTarget = target;
    }, FOCUS_DELAY_MS);

    target.blur();
  }

  function schedule(delay, force = false) {
    clearTimeout(settleTimer);
    settleForce = settleForce || force;
    settleTimer = setTimeout(() => {
      settleTimer = 0;
      const shouldForce = settleForce;
      settleForce = false;
      reanchor(shouldForce);
    }, delay);
  }

  function waitForPointerUp() {
    if (waitingForPointerUp) return;
    waitingForPointerUp = true;
    const done = () => {
      window.clearTimeout(fallback);
      waitingForPointerUp = false;
      window.removeEventListener("pointerup", done, true);
      window.removeEventListener("pointercancel", done, true);
      schedule(0);
    };
    const fallback = window.setTimeout(done, POINTER_WAIT_FALLBACK_MS);
    window.addEventListener("pointerup", done, true);
    window.addEventListener("pointercancel", done, true);
  }

  function onPointerDown() {
    pointerDown = true;
    // 창 포커스/크기 변경으로 예약된 보정은 사용자의 클릭이 끝난 뒤로
    // 미룹니다. 클릭이 편집 요소로 이어지면 focusin이 다시 예약하고,
    // 도구막대 버튼처럼 포커스가 그대로인 경우에도 이 경로가 보정합니다.
    if (settleTimer) {
      clearTimeout(settleTimer);
      settleTimer = 0;
      waitForPointerUp();
    }
  }

  function onPointerUp() {
    pointerDown = false;
  }

  function onFocusIn(event) {
    if (restoring) return;
    const target = event.target;
    if (!isTextEditable(target)) return;

    // 드래그로 텍스트를 선택하는 중이라면 손을 뗀 뒤에 보정합니다.
    if (pointerDown) {
      waitForPointerUp();
    } else {
      schedule(0);
    }
  }

  function onWindowFocus() {
    if (pointerDown) {
      waitForPointerUp();
      return;
    }
    schedule(WINDOW_FOCUS_SETTLE_MS);
  }

  function onWindowBlur() {
    // 창이 비활성화되면 진행 중이던 조합은 어차피 취소됩니다. compositionend가
    // 오지 않는 경우에도 보정이 멈추지 않도록 상태를 초기화합니다.
    clearTimeout(settleTimer);
    settleTimer = 0;
    settleForce = false;
    cancelFocusTimer();
    composing = false;
    pending = false;
  }

  function onResize() {
    // 창 크기 조절 후에도 같은 문제가 생기므로 크기가 안정된 뒤 보정합니다.
    schedule(RESIZE_SETTLE_MS, true);
  }

  function onVisibilityChange() {
    if (document.visibilityState === "visible") schedule(WINDOW_FOCUS_SETTLE_MS, true);
  }

  function onCompositionStart() {
    composing = true;
  }

  function onCompositionEnd() {
    composing = false;
    if (pending) {
      pending = false;
      schedule(0, true);
    }
  }

  requestReanchor = () => schedule(0, true);

  window.addEventListener("focus", onWindowFocus);
  window.addEventListener("blur", onWindowBlur);
  window.addEventListener("resize", onResize);
  window.addEventListener("pointerdown", onPointerDown, true);
  window.addEventListener("pointerup", onPointerUp, true);
  window.addEventListener("pointercancel", onPointerUp, true);
  document.addEventListener("focusin", onFocusIn, true);
  document.addEventListener("visibilitychange", onVisibilityChange);
  document.addEventListener("compositionstart", onCompositionStart, true);
  document.addEventListener("compositionend", onCompositionEnd, true);

  return () => {
    clearTimeout(settleTimer);
    cancelFocusTimer();
    requestReanchor = () => {};
    window.removeEventListener("focus", onWindowFocus);
    window.removeEventListener("blur", onWindowBlur);
    window.removeEventListener("resize", onResize);
    window.removeEventListener("pointerdown", onPointerDown, true);
    window.removeEventListener("pointerup", onPointerUp, true);
    window.removeEventListener("pointercancel", onPointerUp, true);
    document.removeEventListener("focusin", onFocusIn, true);
    document.removeEventListener("visibilitychange", onVisibilityChange);
    document.removeEventListener("compositionstart", onCompositionStart, true);
    document.removeEventListener("compositionend", onCompositionEnd, true);
  };
}
