// Pandoc + Typst Letter Template
// Usage: pandoc letter.md --pdf-engine=typst --template=letter.typ -o letter.pdf
//
// Frontmatter variables:
//   sender-name, sender-address, sender-city, sender-email, sender-phone
//   recipient-name, recipient-address, recipient-city
//   subject, date, closing (default: "Sincerely")

#set page(
  paper: "$if(papersize)$$papersize$$else$a4$endif$",
  margin: (top: 2.5cm, bottom: 2.5cm, left: 2.5cm, right: 2.5cm),
)

#set text(
  font: "$if(mainfont)$$mainfont$$else$Libertinus Serif$endif$",
  size: $if(fontsize)$$fontsize$$else$11pt$endif$,
  lang: "$if(lang)$$lang$$else$en$endif$",
)

#set par(justify: true, leading: 0.65em)

// --- Sender block ---

$if(sender-name)$
#align(right)[
  #text(weight: "bold")[$sender-name$] \
  $if(sender-address)$$sender-address$ \$endif$
  $if(sender-city)$$sender-city$ \$endif$
  $if(sender-email)$$sender-email$ \$endif$
  $if(sender-phone)$$sender-phone$$endif$
]
$endif$

#v(1cm)

// --- Date ---

#align(right)[
  $if(date)$$date$$else$#datetime.today().display("[month repr:long] [day], [year]")$endif$
]

#v(1cm)

// --- Recipient block ---

$if(recipient-name)$
$recipient-name$ \
$if(recipient-address)$$recipient-address$ \$endif$
$if(recipient-city)$$recipient-city$$endif$
$endif$

#v(1cm)

// --- Subject ---

$if(subject)$
#text(weight: "bold")[Re: $subject$]
#v(0.5cm)
$endif$

// --- Body ---

$body$

// --- Closing ---

#v(1cm)
$if(closing)$$closing$$else$Sincerely$endif$,

$if(sender-name)$
#v(1.5cm)
$sender-name$
$endif$
