// Tips: data-tip="Text" on an element shows the text next to it on mouse
// hover and keyboard focus. One element shows them, with fixed position, so
// no container that clips its content can cut a tip off, and it stays inside
// the window: under the element, or above it where there's no room below.
// An icon-only button still needs an aria-label, since screen readers don't
// read tips, and touch screens don't show them.

const GAP = 6;
const MARGIN = 8;
const DELAY = 250;

let bubble = null;
let shownFor = null;
let timer = null;
let watcher = null;

function hide() {
    clearTimeout(timer);
    watcher?.disconnect();
    shownFor = null;
    bubble?.remove();
}

function place(el) {
    const text = el.getAttribute("data-tip");
    if (!text || !el.isConnected) {
        hide();
        return;
    }
    if (!bubble) {
        bubble = document.createElement("div");
        bubble.className = "tip-bubble";
        bubble.setAttribute("role", "tooltip");
        bubble.dataset.cy = "tip";
    }
    bubble.textContent = text;
    // A tip inside an open dialog goes in the dialog, which sits above the page
    const host = el.closest("dialog[open]") || document.body;
    if (bubble.parentElement !== host) {
        host.append(bubble);
    }
    const r = el.getBoundingClientRect();
    const w = bubble.offsetWidth;
    const h = bubble.offsetHeight;
    const vw = document.documentElement.clientWidth;
    const left = Math.min(Math.max(MARGIN, r.left + r.width / 2 - w / 2), vw - w - MARGIN);
    let top = r.bottom + GAP;
    if (top + h > window.innerHeight - MARGIN) {
        top = r.top - GAP - h;
    }
    bubble.style.left = `${Math.max(MARGIN, left)}px`;
    bubble.style.top = `${Math.max(MARGIN, top)}px`;
}

function show(el, now) {
    if (el === shownFor) {
        return;
    }
    hide();
    shownFor = el;
    // A click often changes the tip, e.g. "Taller" to "Smaller"
    watcher = new MutationObserver(() => place(el));
    watcher.observe(el, { attributes: true, attributeFilter: ["data-tip"] });
    if (now) {
        place(el);
    } else {
        timer = setTimeout(() => place(el), DELAY);
    }
}

function tipOf(target) {
    return target instanceof Element ? target.closest("[data-tip]") : null;
}

export function installTips() {
    // pointermove, not pointerover: after a scroll hid the tip, or moved an
    // element under a still pointer, the next movement shows it again
    document.addEventListener("pointermove", (event) => {
        if (event.pointerType !== "mouse") {
            return;
        }
        const el = tipOf(event.target);
        if (el) {
            show(el, false);
        } else if (shownFor) {
            hide();
        }
    });
    document.addEventListener("pointerout", (event) => {
        if (shownFor && !shownFor.contains(event.relatedTarget)) {
            hide();
        }
    });
    document.addEventListener("focusin", (event) => {
        const el = tipOf(event.target);
        if (el && event.target.matches(":focus-visible")) {
            show(el, true);
        }
    });
    // Only the element that showed the tip losing focus hides it: a click
    // elsewhere moves focus away from whatever had it
    document.addEventListener("focusout", (event) => {
        if (shownFor?.contains(event.target)) {
            hide();
        }
    });
    document.addEventListener("keydown", (event) => event.key === "Escape" && hide());
    // Only a scroll that moves the tip's element hides it: a log that follows
    // its end scrolls all the time, elsewhere on the page
    document.addEventListener(
        "scroll",
        (event) => {
            if (shownFor && (event.target === document || event.target.contains?.(shownFor))) {
                hide();
            }
        },
        { capture: true, passive: true },
    );
    window.addEventListener("resize", hide);
}
