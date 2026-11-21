# saundo landing page

A static, bilingual landing page for saundo: one HTML file, self-hosted
fonts, no framework, no bundler, no analytics. It is a marketing surface, not
part of the app build.

## Run it

Serve the folder. Do not open `index.html` from `file://`: the self-hosted
Geist fonts are blocked by the file origin's CORS rules, and the page would
silently fall back to a system font.

```bash
python3 -m http.server 8000 --directory site
```

Then open <http://localhost:8000>.

The page follows the system language (Simplified Chinese or English) and the
system appearance. The header switches both; the choice is kept in
`localStorage`. Nothing else is stored or sent.

## What is here

`index.html` carries the markup, CSS, JavaScript, and the inline SVG symbols.
`assets/` carries only what the page loads: the app icon, the two
ambient-light WebP images, the two Geist fonts, and the JPG social card.
`assets/src/` carries the working sources, the PNG base images plus the
`og.png`/`og.jpeg` exports. The page never requests anything from `assets/src/`.

`site/.audit/` holds local screenshots and probe scripts, and
`site/motion-lab.html` is a local design lab. Both are ignored by git and
are not part of the published page.

The fonts are Geist and Geist Mono by Vercel, under the SIL Open Font
License 1.1. The license text is in `LICENSES/OFL-1.1-Geist.txt` and the
attribution is in `NOTICE`.

## Design notes

The hero is the tray flyout, not a screenshot. It opens on a live two-member
session — glow already up, media paused, volume low — and when the panel sits
in view it runs one scripted pass (play → Standard → both row volumes → next
track); any touch on the panel or the lamp controls hands control back, and
`?demo=0` skips the run. It still answers the visitor: every member row is a
compact mixer with its own volume and mute, the single receiver carries an
icon-only channel button, and rows connect, disconnect, or rejoin while the
rest keep playing. The head's 全部断开 ends the session; removing one member
leaves a notice under the device card with a retry, and adding one shows the
join notice. The footer names the joined session, the way `ConnectedName`
does.
The tray media card and the bento media card render one shared media
session: skipping tracks on either card updates both, and in cover mode the
lamp palette follows the new artwork (five demo tracks, each with its own
poster palette). The glow is gated on lamp × session — turning the lamp off
or ending the session fades it, and it returns with the state. The
latency mode is one state behind the tray segment and
the bento rail — either surface switches, both follow, and the ALAC line
updates at once. The whole visual renders at `zoom: .85` on `.hero-visual` — panel, glow
and caption scale as one object, and the layout height shrinks with it
(`transform: scale` would strand empty space below). The real HomePod photograph belongs only to the ambient-light section:
it is the render target, and the pod itself is a click target that cycles
the light. On desktop the pod sits on the left; the right column carries the
copy, the current name and source chip, the power switch, and seven swatches
(Cover plus the six templates). The copy starts level with the top of the pod,
not centred against it.

`amb-base*.webp` carries an 11% transparent border on every side, so a 480px
pod shows a 374px speaker. On desktop the pod is pulled up into that border
(`margin: calc(var(--pod) * -0.1104)`), which takes the dead band out of the
section; below 1021px the layout stacks, the border is left in place as
breathing room, and the copy comes first with the controls under the pod.

The copy block is centred in the space right of the pod
(`justify-self: center`, `max-width: min(460px, 100%)`). Left-aligned it
stranded ~180px of empty width on the right — far more than the sections above
leave — because the copy is only ~380px of ink wide. Pod size and that right
margin trade off directly: a smaller pod means a wider right margin, so the
pod stays at 480px unless the section layout changes.

The ambient light uses the same three colors and template seeds as
`internal/lamp/presets.go`. The left-hand picker, current name/chip, and
visible swatches follow the earlier design exploration.
The pod keeps the page's original translucent glow layer, and a soft aura
outside the pod carries the current palette.

The disc geometry is measured off the base photos, not the device: the glass
surface ends at 40.3% of the pod box in light and 40.7% in dark, the glass's
own rim shadow runs out to 42.7%, and the mesh starts there. The 43% display
therefore lands on the mesh boundary, so the glow's edge and the glass's edge
are the same circle. What made them read apart was `.amb-pod::after`: a
near-black vignette whose darkest stop (46%) sits on that rim, which on the
white glass reads as a grey backing disc and darkens the rim it covers by
about 20 levels. Light mode now skips the vignette entirely and lets the
glow's own falloff end on the glass; dark mode keeps it, where it is
invisible on the dark fabric and still grounds the light. The display mask
starts at 88% instead of 58%, so the mask no longer cuts the outer glow
before the canvas falloff has finished.

The page mixes card sections (steps) with a bento feature grid, a flat trust
section, and the FAQ, so it does not read as a pile of rounded boxes. The
trust section sits right after the device strip — identity first, then how-to
steps. The
feature block is five cards: ambient light, the media card, latency,
receivers/pairing, and a full-width band for multi-device playback. Each card
carries one small motion; the latency rail is the only interactive one. The
multi-device band is a diagram, not another row list: the app sits at the
centre, two HomePods and two minis at the corners, and one pulse leaves the
centre while all four links carry it on the same beat. It is there for
simultaneity; the hero owns the mixer rows and the loss notice. The clock-sync
claim lives on the latency card
as a small `PTP 时钟同步` chip under the rail — there is no separate sync
section.

The stereo-combo section sits between the feature block and the ambient demo,
and its copy owns the claim the rest of the page does not repeat: Apple's
stereo group needs two of the same model, while a saundo stereo combo links
any two AirPlay 2 receivers across models and generations. The left column is
the heading, the lead, and two chips (one receiver per channel, swap sides
live), with no eyebrow and no fine print. The right side is a stage, not a
card, and it shows the real topology: a HomePod and a HomePod mini stand apart
on one axis with no receiver-to-receiver link, because the combo is two
independent AP2 sessions, not a native pair. The stage is static; it does not
rotate models. The icon-only swap control sends one green signal down both
segments: the first fill and its endpoint dots stay, a stream of smaller dots
keeps running, and both bodies, the dots, and the line glow follow one
2.4-second four-beat level envelope. Each packet arrival adds a small base
ripple and a brief L/R watermark lift; two neutral sound rings leave both
bodies once per bar; a manual swap flashes both lit lines once without breaking
or redrawing them. Reduced motion stops the signal. It carries no channel
words, no text on the swap control, no volume, and no launch switch.
`立体声组` / stereo group is the official pair; `立体声组合` / stereo combo is
the software one. Keep the two words apart in both languages; the FAQ uses the
same pair.

Each claim has one owner, and the sections that follow do not restate it. The
feature cards carry the claims; the ambient section is the demo, so it is a
heading, the controls, and one line for the HomePod-only requirement. The
first-launch and platform limits live only in the FAQ ("第一次打开被系统拦
下来，怎么办", "为什么接收器列表是空的", "哪些设备能连"); the hero note deep
links to the first one (`#install` on the details element) and a small script
opens the hashed entry, so keep that id on it. The hero proves the platform
list once; the trust section does not repeat it. When adding a section, check
whether the sentence already exists elsewhere before writing it again.

Device and receiver names render in English on both pages ("Living Room
HomePod", "Stereo pair", "Mac receiver"); do not add them back to the i18n
dictionary or the zh page will translate them again. The marquee's edge fade
is two static gradients, not `mask-image`: a mask on the container of an
animated track is re-composited every frame and flickers on phones while
scrolling. The container has no `gap`; each track's own `padding-right` is the
inter-track spacing. A track holds one copy of the list, and the script appends
tracks until `(count - 1) * trackWidth` covers the viewport, at least two. Do
not double the content inside a track. Two copies make a 2261 px track, 6783
device px at DPR 3, and on iPhone the strip flickers while scrolling and paints
incompletely at rest. One copy per track is 1131 px, 3393 device px, and renders
correctly (device test 2026-11-15).

Chinese is the source copy; English is a parallel rewrite, not a literal
translation. Image alt text and ARIA labels are translated too. Reduced
motion is honoured; the FAQ uses native `details`.

## Modern web behavior

The document declares `color-scheme` and a `theme-color` that follows the
header toggle, and sets the light/dark palette on the root element before
first paint. Scroll reveals are opt-in through a `js` class: if a script
throws, the class is removed and the copy stays visible; `<noscript>` covers
a disabled script. While the mobile menu is open, `main`, the footer, and the
skip link are `inert`; the closed dialog is inert too, and a viewport change
back to the desktop layout closes it. Tray and lamp segments, the swatch
grid, and the lamp disclosure expose `aria-pressed`/`aria-expanded`; the
language button's accessible name contains its visible text.

The flyout's muted labels are below WCAG AA in the shipped light palette
(3.1–3.7:1 on the panel). Changing that palette would change the tray design,
so it is left for a design decision; `prefers-contrast: more` darkens the
muted tokens for visitors who ask for more contrast.

The mobile menu stays a custom dialog rather than a native `<dialog>`: its
visible nav button doubles as the close control, and a native modal would
make that button inert. It uses a focus trap plus `inert` on the page. The
receiver marquee pauses on hover and stops under reduced motion, but has no
visible pause control; WCAG 2.2.2 would require one, and the current header
and strip design has no slot for it.

## Performance

The ambient glow renders one 2D canvas per frame. Its ring lookup table
(distance, angle, falloff, phase, amplitude) is rebuilt only when the canvas
size changes, the noise offsets and palette interpolation reuse the same
`ImageData`, and the per-pixel noise is inlined. The renderer produces the
same pixels as the first version: a fixed 90-frame run at DPR 2 hashes to
the same value before and after the change. The inlined noise floors its cell
index rather than truncating it: the second sample's drift crosses zero about
24 s after load, and truncation painted hard radial facets from then on. The
canvas stops drawing through an `IntersectionObserver` when the ambient
section leaves the viewport, and the media clock does not advance while the
tab is hidden.

Measured on the development machine with 4x CPU throttling, glow visible, and
`?demo=0`: 10.5 FPS before and 59.5 FPS after at DPR 1; 10–12 FPS before and
34–41 FPS after at DPR 2. On a warm local server at normal CPU, FCP is 192 ms,
LCP is 208 ms, CLS is 0, and the page makes three subresource requests (two
fonts and the icon).

## Deploy

`.github/workflows/deploy-landing.yml` publishes the site to GitHub Pages on
pushes to `main` that touch `site/index.html`, `site/assets/`, the Geist
license, the `NOTICE`, or the workflow. It stages only the shipped files
(`index.html` and `assets/`, without `assets/src/`) plus the font license and
`NOTICE`, uploads the artifact, and deploys it. The repository's Pages source
must be GitHub Actions; the workflow asks `actions/configure-pages` to enable
it on the first run. If that step is rejected, set Settings → Pages → Build
and deployment → Source to GitHub Actions once and re-run the workflow. The
site is <https://4202217.github.io/saundo/>.

## Before publishing

The canonical link, `og:url`, `og:image`, and `twitter:image` point at
`https://4202217.github.io/saundo/`. Change them if a custom domain replaces
the Pages host.

Confirm the provenance and license of `assets/src/amb-base*.png` and
`assets/amb-base*.webp` before publishing. They are project visual assets and
are not covered by the repository's GPL notice. The Geist fonts are covered by
`LICENSES/OFL-1.1-Geist.txt`; the Apple product names are covered by the
trademark disclaimer in the footer.

The page links to the public releases page; no installer is bundled. The
landing page adds no network calls of its own. The Windows build checks
GitHub for signed updates; the macOS build does the same from its signed
feed.

## Checks

Rendered at 1440 and 1024 px in both themes and both languages, and at 390
and 320 px in both languages. The console is clean when served over HTTP.
There is no horizontal overflow at those widths; at 320 the long English
`System Settings → Privacy & Security` label wraps instead of pushing the page
wide. The English and Chinese titles, descriptions, OG tags, alt text, and
ARIA labels switch together, and no visible string stays Chinese on the
English page except the wordmark and the language toggle.

The marquee was probed at 390, 1024, 1440, 1920, and 2560 px. The script
emits 2, 2, 3, 3, and 4 tracks, every sampled animation position covers the
viewport, and WebKit screenshots at the loop boundary show no seam
(`.audit/probe-marquee.mjs`, a local script that is ignored by git).

Clicked through in a headless browser: receiver connect, play and pause, the
four latency modes, a template swatch, the lamp power, and an FAQ entry. The
mobile menu opens with its download action visible and returns focus to its
button when closed. A `#install` deep link opens the hashed FAQ entry.

The multi-device flyout was clicked through as well: disconnecting one member
leaves the loss notice with its retry while the rest keep playing, the head's
全部断开 ends the session, and a reconnect from idle opens one target. Per-row
volume, mute, and the channel cycle all work; a disconnected single receiver's
second line follows the channel with the long word (立体声 / 左声道 / 右声道,
Stereo / Left channel / Right channel). The stereo stage keeps the HomePod/mini
pair static; the icon-only control trades the sides without breaking the lit
line. The line fills once, streams dots, and drives both bodies, the dots, and
the line glow from one 2.4s level envelope; arrival ripples, watermark lifts,
the per-bar sound rings, and the one-shot swap flash are the added layers. The
new cards and the stage layout were checked at 1440, 1020, 390, and 320 px in
Chinese and English, and in both themes; nothing overflows its column. The
final animation layers were not re-checked in a browser.

Lighthouse on the local server (`?demo=0&theme=light&lang=en`): desktop
accessibility 92, best practices 100, SEO 100, agentic browsing 100; mobile
accessibility 96, best practices 100, SEO 100. The desktop failures are the
flyout's muted-text contrast (see above) and the transport buttons' measured
20.4 px boxes. Those buttons carry transparent `::after` hot zones that
extend the target to 36 px before the 0.85 zoom (30.6 px after), and mobile
passes target size, so the 20.4 px reading is a tooling limit.
`prefers-contrast: more` was checked to darken the muted flyout tokens.

With `prefers-reduced-motion: reduce`, the ambient canvas draws one static
frame, the lamp switch clears it, and the hero pass is cancelled by pointer
or keyboard focus. The mobile menu was checked open, close, Escape, link
navigation, and the resize back to the desktop layout; `main`, the footer,
and the skip link follow the `inert` state and the body scroll lock is
restored.
