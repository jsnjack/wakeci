// Colours of task output in the log viewer. The 16 basic colours are CSS
// variables (log.css), one palette for light and one for dark. Others keep
// the program's colour, moved toward the text colour until it has 4.5:1
// contrast with the log's background, so white text on the light theme
// turns grey.

const BACKGROUND = { light: [255, 255, 255], dark: [15, 14, 17] };
const TEXT = { light: [28, 27, 30], dark: [230, 225, 230] };

function luminance([r, g, b]) {
    const channel = (c) => {
        const s = c / 255;
        return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
    };
    return 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b);
}

export function contrast(a, b) {
    const [x, y] = [luminance(a), luminance(b)].sort((p, q) => q - p);
    return (x + 0.05) / (y + 0.05);
}

const cache = new Map();

// readableColor returns rgb as a CSS colour readable on the theme's log
export function readableColor(rgb, theme) {
    const key = `${theme}:${rgb.join(",")}`;
    if (!cache.has(key)) {
        const background = BACKGROUND[theme];
        const text = TEXT[theme];
        let color = rgb;
        for (let step = 1; step <= 10 && contrast(color, background) < 4.5; step++) {
            color = rgb.map((c, i) => Math.round(c + (text[i] - c) * (step / 10)));
        }
        cache.set(key, `rgb(${color.join(" ")})`);
    }
    return cache.get(key);
}

// backgroundColor draws a background at about a quarter of its strength,
// so text on it stays readable in both themes
export function backgroundColor(rgb) {
    return `rgb(${rgb.join(" ")} / 0.25)`;
}
