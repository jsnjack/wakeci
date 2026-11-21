# Styles

The frontend has a stylesheet of its own, with no CSS framework. This page
says how it is built, how to add to it and why it looks as it does.

## Files and layers

`src/frontend/src/assets/styles.css` loads the rest in two cascade layers,
lowest first:

| Layer | File | What it holds |
|---|---|---|
| base | `base.css` | Element defaults for the page around the app: the body, thin scrollbars, the browser's controls in fields |
| app | `tokens.css` | Colours (light and dark), spacing, corners, type scale, the system font, status colours |
| app | `components.css` | The components: layout, buttons, fields, segmented control, tabs, tooltips, dialogs, menus, status icons, notifications, the icon font |
| app | `log.css`, `charts.css` | Colours of task logs and of the analytics charts, loaded by the pages that use them |

A component's own `<style>` is in no layer, so it wins over both without
`!important`. CodeMirror (the job editor and the log viewer) adds its own
styles in no layer too, so style its parts with `EditorView.theme`, not with
layered CSS.

## The `.ui` scope

The classes of `components.css` work only inside an element with the class
`ui`, and every page, the header and the notifications have it on their
root. Inside it, every element except SVG starts from the browser's styles
(`all: revert`), so a component sets everything it needs and nothing leaks
in from elsewhere. SVG keeps its own styles, since rolling it back would also
drop the fill and stroke attributes that draw the charts.

The scope began as the way to move pages off BeerCSS one at a time, which
the app used until November 2026. It stayed because it keeps components
predictable.

- The `.ui` element is positioned, so what a page places absolutely stays
  inside it.
- The layout classes (`stack`, `cluster`, `cols`, `gap-*`) also work on the
  `.ui` element itself, which is where pages put them.
- Spacing comes from `gap`, not from margins.
- A field is a `.form-field` with a `<label for>` above its control and an
  optional `.hint` under it, outside the label, so the label names only its
  field.

The Vue components built on these classes are in `src/components/ui/`
(`UiDialog` on the native `<dialog>`, `UiMenu`) and next to them
(`StatusIcon`, `JobTabs`, `RunDialog`, `LogViewer`).

## Icons

Icons are Material Symbols Outlined by ligature name:
`<i class="icon">search</i>`. The font in `src/assets/fonts/` has only the
icons the app uses, about 10 kB instead of the 464 kB of the whole font.
After adding or removing an icon, run:

```bash
cd src/frontend && npm run icons
```

`scripts/icons.mjs` finds the names in the source and has Google Fonts cut
the font to them, keeping the FILL axis that the status icons use. It finds
a name written in one of these ways:

- the text of `<i class="icon">name</i>`, or what a `{{ }}` there results
  in, such as `{{ open ? "expand_less" : "expand_more" }}`
- `icon: "name"`, `icon="name"`, or `default: "name"` of an `icon` prop
- the values of an object named `ICONS` or `..._ICONS`

A name built in another way is missed and shows as text.
`e2e/icons.spec.js` fails while the font lacks a name the script finds.

## Checking a change

`make screens` takes screenshots of every page at 390, 1024 and 1440 px, in
light and dark, and compares them with master's, or with another commit's
(`BASE=<commit>`). The differences show in `make testreport`, at every width
of a page that changed. E2E tests find elements by test ID (`data-cy`), not
by markup, so markup can change freely.

## The look

Material's colours at the density of a developer tool.

- **Colour.** Material's tonal palette from wakeci's purple, so hover,
  selection, light and dark all follow from the same roles. Dark follows the
  system unless the user picks one. Colour means status only, always with a
  shape (a filled check, cross or clock; a grey outline for queued and not
  run; a turning ring while running). One accent colour.
- **Shape and density.** 12 px corners on containers, 8 to 10 px on
  controls, pills only for navigation; thin dividers instead of cards and
  shadows; text about 15 px. Material's own components are sized for touch
  (40 px chips, 48 px targets), and fit far fewer builds on a screen.
- **Type.** The system font: no download, and familiar on every platform.
- Tried and not chosen: Material 3 Expressive (28 px corners, list items as
  separate tiles, bolder titles) fitted 10 builds on a screen and looked
  playful; a neutral grey style like GitHub's fitted 13 but lost the wakeci
  look.
- Not Tailwind, which rewrites every class use and adds a build plugin, nor
  a component kit, which is another lock-in with larger chunks.
