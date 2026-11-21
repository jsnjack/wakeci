import { test, expect } from "./fixtures.js";
import { iconNames } from "../scripts/icons.mjs";

test("icon names are found however the source writes them", () => {
    const names = iconNames();
    // In <i class="icon">, a ternary there, an icon key, an icon prop, a
    // prop's default and an ICONS map
    for (const name of ["water", "dark_mode", "monitoring", "menu", "more_horiz", "radio_button_unchecked"]) {
        expect(names).toContain(name);
    }
    // What a ternary compares is not an icon
    expect(names).not.toContain("light");
});

test("the icon font has every icon the source uses", async ({ page }) => {
    await page.goto("/");
    const missing = await page.evaluate(async (names) => {
        const box = document.createElement("div");
        box.className = "ui";
        for (const name of names) {
            const icon = document.createElement("i");
            icon.className = "icon";
            icon.textContent = name;
            box.append(icon);
        }
        document.body.append(box);
        await document.fonts.load('20px "Material Symbols Outlined"');
        // An icon is one square glyph as wide as the font size; a name the
        // font lacks shows as its letters
        return [...box.children].filter((icon) => icon.getBoundingClientRect().width !== 20).map((icon) => icon.textContent);
    }, iconNames());
    expect(missing, "the font lacks these icons: run npm run icons").toEqual([]);
});
