# Miscellaneous Diagrams

Syntax reference for BPMN, Excalidraw, Ditaa, SvgBob, Erd, DBML, WaveDrom, Bytefield, WireViz, Pikchr, TikZ, and UMLet.

---

## Erd — Entity-Relationship Diagrams

```erd
[Customer]
*id
name
email
phone

[Order]
*id
+customer_id
date
total

[Product]
*id
name
price
stock

[OrderItem]
*id
+order_id
+product_id
quantity

Customer 1--* Order
Order 1--* OrderItem
Product 1--* OrderItem
```

Syntax: `*` = primary key, `+` = foreign key, `1--*` one-to-many, `1--1` one-to-one, `*--*` many-to-many

---

## DBML — Database Markup Language

```dbml
Table users {
    id integer [pk, increment]
    username varchar [unique, not null]
    email varchar [unique, not null]
    created_at timestamp [default: `now()`]
}

Table posts {
    id integer [pk, increment]
    title varchar [not null]
    body text
    user_id integer [ref: > users.id]
    status post_status
    created_at timestamp
}

Table comments {
    id integer [pk, increment]
    post_id integer [ref: > posts.id]
    user_id integer [ref: > users.id]
    body text [not null]
}

Enum post_status {
    draft
    published
    archived
}
```

Relationships: `ref: >` many-to-one, `ref: <` one-to-many, `ref: -` one-to-one

---

## Ditaa — ASCII Art to Diagrams

```ditaa
+--------+   +--------+   +--------+
|  User  |-->| Server |-->|  DB    |
|  cBLU  |   | cGRE   |   | cPNK   |
+--------+   +--------+   +--------+
                 |
                 v
             +--------+
             | Cache  |
             | cYEL   |
             +--------+
```

Colors: `cBLU` blue, `cGRE` green, `cPNK` pink, `cYEL` yellow, `cRED` red, `cBLK` black

Special chars: `--` dashed lines, `=` thick lines, `*` bullets, `{d}` document shape, `{s}` storage shape, `{io}` I/O shape, `{o}` ellipse

---

## SvgBob — ASCII Art to SVG

```svgbob
       .---.
      /     \
     |  App  |
      \     /
       '---'
         |
    .----+----.
    |         |
    v         v
 +-----+  +-----+
 | API |  | Web |
 +--+--+  +--+--+
    |         |
    v         v
 +---------------+
 |   Database    |
 +---------------+
```

SvgBob auto-detects ASCII art patterns: boxes (`+--+`), lines (`|`, `-`, `/`, `\`), arrows (`>`, `v`, `<`, `^`), curves (`.`, `'`), circles (`o`, `O`, `*`)

---

## BPMN — Business Process Model

BPMN uses XML. Basic structure:

```bpmn
<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:bpmndi="http://www.omg.org/spec/BPMN/20100524/DI"
             xmlns:dc="http://www.omg.org/spec/DD/20100524/DC"
             xmlns:di="http://www.omg.org/spec/DD/20100524/DI"
             id="def" targetNamespace="http://example.com">
    <process id="process1" isExecutable="false">
        <startEvent id="start" name="Start"/>
        <task id="task1" name="Review Request"/>
        <exclusiveGateway id="gw1" name="Approved?"/>
        <task id="task2" name="Process"/>
        <task id="task3" name="Reject"/>
        <endEvent id="end" name="End"/>
        <sequenceFlow sourceRef="start" targetRef="task1"/>
        <sequenceFlow sourceRef="task1" targetRef="gw1"/>
        <sequenceFlow sourceRef="gw1" targetRef="task2" name="Yes"/>
        <sequenceFlow sourceRef="gw1" targetRef="task3" name="No"/>
        <sequenceFlow sourceRef="task2" targetRef="end"/>
        <sequenceFlow sourceRef="task3" targetRef="end"/>
    </process>
    <bpmndi:BPMNDiagram id="diagram1">
        <bpmndi:BPMNPlane bpmnElement="process1">
            <bpmndi:BPMNShape bpmnElement="start"><dc:Bounds x="50" y="100" width="36" height="36"/></bpmndi:BPMNShape>
            <bpmndi:BPMNShape bpmnElement="task1"><dc:Bounds x="150" y="80" width="100" height="80"/></bpmndi:BPMNShape>
            <bpmndi:BPMNShape bpmnElement="gw1" isMarkerVisible="true"><dc:Bounds x="300" y="95" width="50" height="50"/></bpmndi:BPMNShape>
            <bpmndi:BPMNShape bpmnElement="task2"><dc:Bounds x="400" y="30" width="100" height="80"/></bpmndi:BPMNShape>
            <bpmndi:BPMNShape bpmnElement="task3"><dc:Bounds x="400" y="140" width="100" height="80"/></bpmndi:BPMNShape>
            <bpmndi:BPMNShape bpmnElement="end"><dc:Bounds x="560" y="100" width="36" height="36"/></bpmndi:BPMNShape>
        </bpmndi:BPMNPlane>
    </bpmndi:BPMNDiagram>
</definitions>
```

Note: BPMN is verbose. For simple process flows, prefer Mermaid flowcharts or PlantUML activity diagrams. Use BPMN only when BPMN-standard compliance is required.

---

## WaveDrom — Digital Timing Diagrams

```wavedrom
{
  "signal": [
    {"name": "clk",  "wave": "p.....|..."},
    {"name": "data", "wave": "x.345x|=.x", "data": ["A", "B", "C", "D"]},
    {"name": "req",  "wave": "0.1..0|1.0"},
    {"name": "ack",  "wave": "1.....|01."}
  ],
  "head": {"text": "SPI Transfer"},
  "config": {"hscale": 2}
}
```

Wave chars: `p`/`n` posedge/negedge clock, `0`/`1` low/high, `x` undefined, `=` data, `.` continue, `2`-`9` colored data, `|` gap

---

## Bytefield — Protocol/Byte Field Diagrams

```bytefield
(defattrs :bg-green {:fill "#a0ffa0"})
(defattrs :bg-yellow {:fill "#ffffa0"})
(defattrs :bg-pink {:fill "#ffb0a0"})

(draw-column-headers)

(draw-box "Version" [:bg-pink {:span 4}])
(draw-box "IHL" [:bg-pink {:span 4}])
(draw-box "DSCP" [:bg-green {:span 6}])
(draw-box "ECN" [:bg-green {:span 2}])
(draw-box "Total Length" [{:span 16}])

(draw-box "Identification" [{:span 16}])
(draw-box "Flags" [:bg-yellow {:span 3}])
(draw-box "Fragment Offset" [:bg-yellow {:span 13}])

(draw-box "TTL" [:bg-pink {:span 8}])
(draw-box "Protocol" [:bg-pink {:span 8}])
(draw-box "Header Checksum" [{:span 16}])

(draw-box "Source IP Address" [{:span 32}])
(draw-box "Destination IP Address" [{:span 32}])
```

---

## WireViz — Wiring Harness Diagrams

WireViz uses YAML input:

```wireviz
connectors:
  X1:
    type: Molex KK 254
    subtype: female
    pinlabels: [GND, VCC, SDA, SCL]
  X2:
    type: Molex KK 254
    subtype: male
    pinlabels: [GND, VCC, SDA, SCL]

cables:
  W1:
    wirecount: 4
    length: 0.3
    color_code: IEC

connections:
  - X1: [1,2,3,4]
    W1: [1,2,3,4]
    X2: [1,2,3,4]
```

---

## Pikchr — PIC-like Technical Diagrams

```pikchr
arrow right 200% "Input" above
box rad 10px "Process" fit
arrow right 200% "Output" above
arrow down 70% from last box.s
box rad 10px "Log" fit
```

Primitives: `box`, `circle`, `ellipse`, `cylinder`, `line`, `arrow`, `arc`, `dot`, `oval`, `text`

Attributes: `right`, `left`, `up`, `down`, `width`, `height`, `radius`, `color`, `fill`, `dashed`, `dotted`, `thick`, `thin`, `fit`

---

## TikZ — LaTeX Diagrams

```tikz
\begin{tikzpicture}[node distance=2cm, auto]
    \node[draw, rounded corners] (start) {Start};
    \node[draw, below of=start] (process) {Process};
    \node[draw, diamond, below of=process] (decision) {OK?};
    \node[draw, rounded corners, below of=decision] (end) {End};
    \node[draw, right of=decision, node distance=3cm] (error) {Retry};

    \draw[->] (start) -- (process);
    \draw[->] (process) -- (decision);
    \draw[->] (decision) -- node{yes} (end);
    \draw[->] (decision) -- node{no} (error);
    \draw[->] (error) |- (process);
\end{tikzpicture}
```

Note: TikZ requires a LaTeX distribution in the Kroki server. Very powerful but verbose. Prefer other diagram types unless LaTeX-quality output is specifically needed.

---

## Excalidraw — Hand-Drawn Style

Excalidraw input is JSON (its native format). Best created using the Excalidraw editor and exported:

```excalidraw
{
  "type": "excalidraw",
  "version": 2,
  "elements": [
    {
      "type": "rectangle",
      "x": 100,
      "y": 100,
      "width": 200,
      "height": 100,
      "strokeColor": "#000000",
      "backgroundColor": "#a5d8ff",
      "fillStyle": "solid",
      "roughness": 1,
      "id": "rect1"
    },
    {
      "type": "text",
      "x": 145,
      "y": 135,
      "text": "Service A",
      "fontSize": 20,
      "id": "text1"
    }
  ]
}
```

Note: Excalidraw JSON is verbose and hard to write by hand. Use it when the user provides Excalidraw exports, or prefer Mermaid/D2 for hand-drawn aesthetics generated from text.
