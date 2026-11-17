// Pandoc + Typst Slide Template (16:9)
// Usage: pandoc slides.md --pdf-engine=typst --template=slide-template.typ -o slides.pdf

#set page(
  paper: "presentation-16-9",
  margin: (top: 1.5cm, bottom: 1.5cm, left: 2cm, right: 2cm),
  footer: context {
    set text(8pt, fill: luma(150))
    h(1fr)
    counter(page).display("1 / 1", both: true)
    h(1fr)
  },
)

#set text(
  font: "$if(mainfont)$$mainfont$$else$Libertinus Serif$endif$",
  size: $if(fontsize)$$fontsize$$else$20pt$endif$,
  lang: "$if(lang)$$lang$$else$en$endif$",
)

#set par(leading: 0.7em)

// --- Title slide ---

#page(footer: none)[
  #align(center + horizon)[
    #text(36pt, weight: "bold")[$if(title)$$title$$endif$]

    $if(subtitle)$
    #v(0.5cm)
    #text(20pt, fill: luma(80))[$subtitle$]
    $endif$

    #v(1cm)

    $if(author)$
    #text(18pt)[$for(author)$$author$$sep$ #sym.dot.c $endfor$]
    $endif$

    $if(date)$
    #v(0.3cm)
    #text(16pt, fill: luma(120))[$date$]
    $endif$
  ]
]

// --- Slide headings ---

#show heading.where(level: 1): it => {
  pagebreak()
  align(center + horizon)[
    #text(32pt, weight: "bold", fill: rgb("#1a73e8"))[#it.body]
  ]
}

#show heading.where(level: 2): it => {
  pagebreak()
  text(28pt, weight: "bold")[#it.body]
  v(0.5cm)
}

#show heading.where(level: 3): it => {
  v(0.3cm)
  text(22pt, weight: "bold")[#it.body]
  v(0.2cm)
}

// --- Code ---

#show raw.where(block: true): it => {
  set text(14pt)
  block(fill: luma(245), inset: 12pt, radius: 6pt, width: 100%, it)
}

// --- Links ---

#show link: it => {
  set text(fill: rgb("#1a73e8"))
  underline(it)
}

// --- Body ---

$body$
