// Pandoc + Typst Invoice Template
// Usage: pandoc invoice.md --pdf-engine=typst --template=invoice.typ -o invoice.pdf
//
// Frontmatter variables:
//   invoice-number, date, due-date
//   company-name, company-address, company-city, company-email
//   client-name, client-address, client-city
//   currency (default: €)

#set page(
  paper: "$if(papersize)$$papersize$$else$a4$endif$",
  margin: (top: 2cm, bottom: 2cm, left: 2cm, right: 2cm),
)

#set text(
  font: "$if(mainfont)$$mainfont$$else$Libertinus Serif$endif$",
  size: $if(fontsize)$$fontsize$$else$10pt$endif$,
  lang: "$if(lang)$$lang$$else$en$endif$",
)

// --- Header ---

#grid(
  columns: (1fr, 1fr),
  align: (left, right),
  [
    #text(24pt, weight: "bold")[INVOICE]
    #v(0.3cm)
    $if(invoice-number)$
    #text(11pt)[*Invoice \#:* $invoice-number$] \
    $endif$
    $if(date)$
    #text(11pt)[*Date:* $date$] \
    $endif$
    $if(due-date)$
    #text(11pt)[*Due:* $due-date$]
    $endif$
  ],
  [
    $if(company-name)$
    #text(14pt, weight: "bold")[$company-name$] \
    $if(company-address)$$company-address$ \$endif$
    $if(company-city)$$company-city$ \$endif$
    $if(company-email)$$company-email$$endif$
    $endif$
  ],
)

#v(1cm)
#line(length: 100%, stroke: 1pt + luma(200))
#v(0.5cm)

// --- Bill To ---

$if(client-name)$
#text(11pt, weight: "bold")[Bill To:]
#v(0.2cm)
$client-name$ \
$if(client-address)$$client-address$ \$endif$
$if(client-city)$$client-city$$endif$
$endif$

#v(1cm)

// --- Body (items table goes here in markdown) ---

$body$

#v(1cm)
#line(length: 100%, stroke: 0.5pt + luma(200))

#v(0.5cm)
#align(center)[
  #text(9pt, fill: luma(120))[
    $if(company-name)$$company-name$$endif$ $if(company-email)$— $company-email$$endif$
  ]
]
