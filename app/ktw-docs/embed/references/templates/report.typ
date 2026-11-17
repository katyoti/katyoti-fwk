// Pandoc + Typst Report Template
// Usage: pandoc input.md --pdf-engine=typst --template=report.typ -o output.pdf

// --- Template variables (populated by pandoc) ---

#let title = "$if(title)$$title$$else$Untitled$endif$"
#let author = "$if(author)$$for(author)$$author$$sep$, $endfor$$endif$"
#let date = "$if(date)$$date$$else$$endif$"
#let subtitle = "$if(subtitle)$$subtitle$$else$$endif$"
#let abstract-text = "$if(abstract)$$abstract$$else$$endif$"
#let lang = "$if(lang)$$lang$$else$en$endif$"

// --- Page setup ---

#set page(
  paper: "$if(papersize)$$papersize$$else$a4$endif$",
  margin: (
    top: $if(margin.top)$$margin.top$$else$2.5cm$endif$,
    bottom: $if(margin.bottom)$$margin.bottom$$else$2.5cm$endif$,
    left: $if(margin.left)$$margin.left$$else$2.5cm$endif$,
    right: $if(margin.right)$$margin.right$$else$2.5cm$endif$,
  ),
  header: context {
    if counter(page).get().first() > 1 [
      #set text(9pt, fill: luma(120))
      #title
      #h(1fr)
      #date
      #line(length: 100%, stroke: 0.5pt + luma(200))
    ]
  },
  footer: context {
    set text(9pt, fill: luma(120))
    line(length: 100%, stroke: 0.5pt + luma(200))
    v(4pt)
    h(1fr)
    counter(page).display("1 / 1", both: true)
    h(1fr)
  },
)

// --- Typography ---

#set text(
  font: "$if(mainfont)$$mainfont$$else$Libertinus Serif$endif$",
  size: $if(fontsize)$$fontsize$$else$11pt$endif$,
  lang: lang,
)

#set par(
  justify: true,
  leading: $if(linestretch)$$linestretch$ * 0.65em$else$0.65em$endif$,
)

// --- Headings ---

#set heading(numbering: "$if(section-numbering)$$section-numbering$$else$1.1.1$endif$")

#show heading.where(level: 1): it => {
  pagebreak(weak: true)
  v(1cm)
  set text(18pt, weight: "bold")
  block(it)
  v(0.5cm)
}

#show heading.where(level: 2): it => {
  v(0.6cm)
  set text(14pt, weight: "bold")
  block(it)
  v(0.3cm)
}

#show heading.where(level: 3): it => {
  v(0.4cm)
  set text(12pt, weight: "bold")
  block(it)
  v(0.2cm)
}

// --- Code blocks ---

#show raw.where(block: true): it => {
  set text(9pt)
  block(
    fill: luma(245),
    inset: 10pt,
    radius: 4pt,
    width: 100%,
    it,
  )
}

// --- Links ---

#show link: it => {
  set text(fill: rgb("$if(linkcolor)$$linkcolor$$else$#1a73e8$endif$"))
  underline(it)
}

// --- Title page ---

#v(3cm)

#align(center)[
  #text(28pt, weight: "bold")[#title]

  #if subtitle != "" [
    #v(0.5cm)
    #text(16pt, fill: luma(80))[#subtitle]
  ]

  #v(1cm)

  #if author != "" [
    #text(14pt)[#author]
  ]

  #if date != "" [
    #v(0.3cm)
    #text(12pt, fill: luma(100))[#date]
  ]
]

#if abstract-text != "" [
  #v(2cm)
  #block(
    inset: (left: 2cm, right: 2cm),
    [
      #text(12pt, weight: "bold")[Abstract]
      #v(0.3cm)
      #text(10pt)[#abstract-text]
    ],
  )
]

#pagebreak()

// --- Table of contents ---

$if(toc)$
#outline(
  title: "Contents",
  depth: $if(toc-depth)$$toc-depth$$else$3$endif$,
  indent: 1.5em,
)
#pagebreak()
$endif$

// --- Document body ---

$body$
