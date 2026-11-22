// The favicon with a dot in the colour of a build's status (L4 in
// docs/ui-roadmap.md), so a build's tab says how it is going

const PLAIN = "/favicon.ico";
const SIZE = 32;

let base = null;
// Counts the changes, so a dot drawn after the page closed doesn't show
let generation = 0;

function link() {
    let icon = document.querySelector("link[rel='icon']");
    if (!icon) {
        icon = document.createElement("link");
        icon.rel = "icon";
        document.head.append(icon);
    }
    return icon;
}

function loadBase() {
    if (!base) {
        base = new Promise((resolve) => {
            const image = new Image();
            image.onload = () => resolve(image);
            image.onerror = () => resolve(null);
            image.src = PLAIN;
        });
    }
    return base;
}

// showStatus puts a dot of the status's colour on the favicon
export async function showStatus(status) {
    const mine = ++generation;
    const image = await loadBase();
    if (mine !== generation) {
        return;
    }
    const colour = getComputedStyle(document.body)
        .getPropertyValue(`--status-${status.replace(" ", "-")}`)
        .trim();
    const canvas = document.createElement("canvas");
    canvas.width = SIZE;
    canvas.height = SIZE;
    const ctx = canvas.getContext("2d");
    if (!ctx || !colour) {
        return;
    }
    if (image) {
        ctx.drawImage(image, 0, 0, SIZE, SIZE);
    }
    // A ring in the page's background keeps the dot apart from the icon
    ctx.beginPath();
    ctx.arc(SIZE - 9, SIZE - 9, 9, 0, 2 * Math.PI);
    ctx.fillStyle = getComputedStyle(document.body).getPropertyValue("--surface").trim() || "#fff";
    ctx.fill();
    ctx.beginPath();
    ctx.arc(SIZE - 9, SIZE - 9, 6.5, 0, 2 * Math.PI);
    ctx.fillStyle = colour;
    ctx.fill();
    const icon = link();
    icon.href = canvas.toDataURL("image/png");
    icon.dataset.status = status;
}

// showPlain puts the plain favicon back
export function showPlain() {
    generation++;
    const icon = link();
    icon.href = PLAIN;
    delete icon.dataset.status;
}
