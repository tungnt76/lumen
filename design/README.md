# Design prototype

`prototype/project/` holds the source of the **Film Site Prototype** canvas made in Claude:
six screens (Home, Movie + player, Browse, Mobile home, Admin sign-in, Admin upload).

These `.dc.html` files are reference only. They render inside the Claude design canvas,
not in a normal browser. The real, runnable UI is the Next.js app in `web/`.

| Screen | Prototype file | Implemented in |
|---|---|---|
| Home | `Main.dc.html` | `web/app/(site)/page.tsx` |
| Movie page + player | `Detail.dc.html` | `web/app/(site)/movie/[id]/page.tsx`, `web/components/Player.tsx` |
| Browse & search | `Browse.dc.html` | `web/app/(site)/browse/page.tsx` |
| Mobile home | `Mobile.dc.html` | responsive CSS in `web/app/globals.css` (`@media (max-width: 900px)`) |
| Admin sign-in | `AdminLogin.dc.html` | `web/app/studio/page.tsx` |
| Admin upload & library | `Admin.dc.html` | `web/app/studio/films/page.tsx` |

Design tokens (also in `web/app/globals.css`):

- Background `#0E0E10`, surface `#151518`, text `#F2EFE9`, muted `#A9A6A0`
- Accent `#E8A33D` (hover `#F4C06F`), text on accent `#1A1206`
- Fonts: Bricolage Grotesque (headings), DM Sans (body)

To change the design: open the canvas from claude.ai/code/artifacts, edit it there (or ask Claude to),
then carry the change into `web/`. Share › Export on the canvas downloads the screens as images or PDF.
