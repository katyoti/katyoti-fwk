// Pandoc + Typst Article Template
// Usage: pandoc input.md --pdf-engine=typst --template=article.typ -o output.pdf
// Clean, minimal layout for papers and blog posts.

#set page(
  paper: "$if(papersize)$$papersize$$else$a4$endif$",
  margin: (
    top: $if(margin.top)$$margin.top$$else$3cm$endif$,
    bottom: $if(margin.bottom)$$margin.bottom$$else$3cm$endif$,
    left: $if(margin.left)$$margin.left$$else$3cm$endif$,
    right: $if(margin.right)$$margin.right$$else$3cm$endif$,
  ),
  footer: context {
    set text(9pt, fill: luma(150))
    h(1fr)
    counter(page).display()
    h(1fr)
  },
)

#set text(
  font: "$if(mainfont)$$mainfont$$else$Libertinus Serif$endif$",
  size: $if(fontsize)$$fontsize$$else$11pt$endif$,
  lang: "$if(lang)$$lang$$else$en$endif$",
)

#set par(justify: true, leading: $if(linestretch)$$linestretch$ * 0.65em$else$0.65em$endif$)

// --- Title block ---

#align(center)[
  #set par(justify: false)
  #text(20pt, weight: "bold", hyphenate: false)[$if(title)$$title$$endif$]

  $if(subtitle)$
  #v(0.2cm)
  #text(13pt, fill: luma(80), hyphenate: false)[$subtitle$]
  $endif$

  #v(0.3cm)

  $if(author)$
  #text(11pt)[$for(author)$$author$$sep$ #sym.dot.c $endfor$]
  $endif$

  $if(date)$
  #v(0.15cm)
  #text(10pt, fill: luma(100))[$date$]
  $endif$
]

$if(abstract)$
#v(0.8cm)
#block(
  inset: (left: 1.5cm, right: 1.5cm),
  stroke: (left: 2pt + luma(200)),
)[
  #set text(10pt)
  #emph[$abstract$]
]
$endif$

#v(0.5cm)

// --- Headings ---
// Keep headings with next content to avoid orphaned titles

#show heading.where(level: 1): it => {
  v(0.5cm)
  set text(15pt, weight: "bold")
  block(below: 0.25cm, sticky: true, it)
}

#show heading.where(level: 2): it => {
  v(0.35cm)
  set text(12.5pt, weight: "bold")
  block(below: 0.2cm, sticky: true, it)
}

#show heading.where(level: 3): it => {
  v(0.2cm)
  set text(11pt, weight: "bold")
  block(below: 0.1cm, sticky: true, it)
}

// --- Figures / Images ---
// Constrain images to max 85% of page height to prevent overflow.
// Keep figures together (non-breakable) so caption stays with image.

#show figure: set block(breakable: false)
#set image(height: auto, width: auto)
#show image: it => {
  block(breakable: false, width: 100%, align(center,
    box(clip: true, it)
  ))
}

// --- Code ---

#show raw.where(block: true): it => {
  set text(9pt)
  block(fill: luma(248), inset: 8pt, radius: 3pt, width: 100%, it)
}

// --- Links ---

#show link: it => {
  set text(fill: rgb("$if(linkcolor)$$linkcolor$$else$#2563eb$endif$"))
  underline(it)
}

// --- TOC ---

$if(toc)$
#outline(title: "Contents", depth: $if(toc-depth)$$toc-depth$$else$3$endif$, indent: 1.5em)
#v(1cm)
$endif$

// --- Horizontal rules ---

#let horizontalrule = line(length: 40%, stroke: 0.5pt + luma(180))

// --- Body ---

$body$
