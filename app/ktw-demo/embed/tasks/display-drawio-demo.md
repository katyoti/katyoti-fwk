---
name: display-drawio-demo
advertised: false
description: Emits an impressive, production-quality .drawio diagram (UML class diagram by default, or sequence / ER / C4 depending on topic) as a kind=drawio artifact. Demonstrates the drawio document type rendering live in ktw-gui with a Save button.
type: task
iconName: vector-triangle
icon: "📐"
hashtags: [demo, drawio, diagram, gui, uml]
category: demo
config:
  capability: smart
  plugins:
    - standard
---

# Drawio Demo

You demonstrate the **drawio document type** added in gui-drawio M1 (2026-05-07).
Emit a **substantial, beautiful diagram** via the `display-ui-component` tool
with `kind: "drawio"`. The user's GUI lazy-loads the vendored Apache-2.0 drawio
viewer and renders the diagram inline; clicking the Save button in the iframe
toolbar downloads the `.drawio` file for editing in their local drawio app.

**This is a showcase task** — the diagram MUST look professional. Realistic
labels, proper drawio styling (compartments, color coding, multiplicity
notation, stereotypes when appropriate), and a clean visual layout. The user
chose this demo to see what an LLM can produce — match that expectation.

## ⚠️ DO NOT EMIT A SEQUENCE DIAGRAM BY DEFAULT

Sequence diagrams (vertical lifelines + horizontal arrows + numbered messages)
are the BORING default that LLMs reach for. **This task explicitly rejects
that pattern.** Default to a UML class diagram unless the user TYPED words
that EXACTLY match sequence-diagram intent ("sequence diagram", "request flow",
"OAuth flow"). For "microservices order flow" or "architecture", use the **C4
component diagram** style — NOT a sequence of HTTP calls between rectangles.

## Step 1 — Read the diagram request from your input

The diagram request is provided in your input — read it from the `FORM_INPUT` block (key `diagram`), or from the message text when run from the CLI. Do NOT use `ask-user`. If empty or absent, default to the UML class diagram.

That value becomes `<topic>`. Pick the **diagram type** by matching
EXACT keywords in it (not vibes):

| Keyword in user input | Use diagram type |
|---|---|
| (none — terse or vague) | **UML class diagram** ✅ default |
| "class", "domain model", "OO design", "entity", "model" | UML class diagram |
| "ER", "database", "schema", "tables" | ER diagram (boxes with column lists, crow's-foot connectors) |
| "microservices", "components", "architecture", "C4", "system design" | **C4 component diagram** (rounded boxes labeled "[Component]" + tech-stack annotations, dashed boundary rects for systems) |
| "state machine", "lifecycle", "states" | State diagram (rounded boxes + initial/final pseudostates) |
| "sequence diagram", "request flow", "auth flow", "OAuth", "saga" | Sequence diagram (lifelines + horizontal arrows) — ONLY when explicitly asked |

**If you're tempted to use sequence diagram for "microservices order flow", STOP.**
The right shape is C4: boxes for `Web App [React]`, `API Gateway [Kong]`,
`Order Service [Go]`, `Order DB [Postgres]`, `Inventory Service [Python]`,
`Payment Gateway [external]`, connected by labeled arrows showing relationships
(`publishes events to`, `reads/writes`, `validates with`), with a dashed
boundary rect grouping the internal services.

## Step 2 — Emit a drawio artifact

Call `display-ui-component` with `kind: "drawio"` and a `data` carrying
the mxGraph XML:

```json
{
  "artifactId": "art-drawio-demo",
  "kind": "drawio",
  "title": "<Topic> — <diagram type>",
  "data": {
    "filename": "<topic-slug>.drawio",
    "xml": "<the full mxfile XML — see templates below>"
  }
}
```

The XML must use the canonical `<mxfile><diagram><mxGraphModel><root>` shape.
Every cell needs a unique `id`, `vertex="1"` or `edge="1"`, a `parent` (usually
`"1"` for top-level), and `<mxGeometry>`. Edges connecting two cells use
`source` / `target` attributes; free-floating edges use `<mxPoint as="sourcePoint"/>`
+ `<mxPoint as="targetPoint"/>`.

---

## Default template — UML Class Diagram (e-commerce domain)

This is the **gold-standard impressive output**. Adapt it for whatever
topic the user picks: rename classes, swap attributes, keep the styling.
The template shows: stereotypes, abstract classes, interfaces, all four
relationship types (inheritance, composition, aggregation, dependency),
multiplicity labels, and a color-coded legend.

```xml
<mxfile host="app.diagrams.net" modified="2026-05-19T00:00:00.000Z" type="device">
  <diagram name="Domain Model" id="dm-1">
    <mxGraphModel dx="1400" dy="1000" grid="1" gridSize="10" guides="1" tooltips="1" connect="1" arrows="1" fold="1" page="1" pageScale="1" pageWidth="1400" pageHeight="1000" math="0" shadow="0">
      <root>
        <mxCell id="0" />
        <mxCell id="1" parent="0" />

        <!-- ─── Title + legend ─────────────────────────────────────────── -->
        <mxCell id="title" value="E-Commerce Domain Model" style="text;html=1;strokeColor=none;fillColor=none;align=center;verticalAlign=middle;whiteSpace=wrap;rounded=0;fontSize=20;fontStyle=1;" vertex="1" parent="1">
          <mxGeometry x="40" y="20" width="600" height="30" as="geometry" />
        </mxCell>
        <mxCell id="legend" value="&lt;b&gt;Legend&lt;/b&gt;&lt;br&gt;&#129095; Entity &amp;nbsp;&amp;nbsp; &#129093; Value object &amp;nbsp;&amp;nbsp; &#129082; Service&lt;br&gt;&lt;b&gt;◇&lt;/b&gt; Aggregation &amp;nbsp; &lt;b&gt;◆&lt;/b&gt; Composition &amp;nbsp; &lt;b&gt;▷&lt;/b&gt; Inheritance &amp;nbsp; &lt;b&gt;⇢&lt;/b&gt; Dependency" style="text;html=1;strokeColor=#cccccc;fillColor=#fafafa;align=left;verticalAlign=middle;whiteSpace=wrap;rounded=1;fontSize=11;spacingLeft=8;spacingRight=8;" vertex="1" parent="1">
          <mxGeometry x="900" y="20" width="460" height="50" as="geometry" />
        </mxCell>

        <!-- ─── Abstract base: Account ─────────────────────────────────── -->
        <mxCell id="account" value="&lt;&lt;abstract&gt;&gt; Account" style="swimlane;fontStyle=3;align=center;verticalAlign=top;childLayout=stackLayout;horizontal=1;startSize=30;horizontalStack=0;resizeParent=1;resizeParentMax=0;horizontalStack=0;collapsible=0;marginBottom=0;swimlaneFillColor=#ffffff;fillColor=#dae8fc;strokeColor=#6c8ebf;fontSize=13;" vertex="1" parent="1">
          <mxGeometry x="80" y="100" width="220" height="130" as="geometry" />
        </mxCell>
        <mxCell id="account-attrs" value="- id: UUID&#xA;- email: string&#xA;- createdAt: DateTime" style="text;strokeColor=none;fillColor=none;align=left;verticalAlign=middle;spacingLeft=8;spacingRight=8;overflow=hidden;rotatable=0;points=[[0,0.5],[1,0.5]];portConstraint=eastwest;fontSize=12;" vertex="1" parent="account">
          <mxGeometry y="30" width="220" height="55" as="geometry" />
        </mxCell>
        <mxCell id="account-sep" value="" style="line;strokeWidth=1;fillColor=none;align=left;verticalAlign=middle;spacingTop=-1;spacingLeft=3;spacingRight=3;rotatable=0;labelPosition=right;points=[];portConstraint=eastwest;strokeColor=#6c8ebf;" vertex="1" parent="account">
          <mxGeometry y="85" width="220" height="8" as="geometry" />
        </mxCell>
        <mxCell id="account-methods" value="+ verify(): bool&#xA;# audit(action: string): void" style="text;strokeColor=none;fillColor=none;align=left;verticalAlign=middle;spacingLeft=8;spacingRight=8;overflow=hidden;rotatable=0;points=[[0,0.5],[1,0.5]];portConstraint=eastwest;fontSize=12;" vertex="1" parent="account">
          <mxGeometry y="93" width="220" height="37" as="geometry" />
        </mxCell>

        <!-- ─── Customer (extends Account) ─────────────────────────────── -->
        <mxCell id="customer" value="&#129095; Customer" style="swimlane;fontStyle=1;align=center;verticalAlign=top;childLayout=stackLayout;horizontal=1;startSize=30;horizontalStack=0;resizeParent=1;resizeParentMax=0;collapsible=0;marginBottom=0;swimlaneFillColor=#ffffff;fillColor=#dae8fc;strokeColor=#6c8ebf;fontSize=13;" vertex="1" parent="1">
          <mxGeometry x="40" y="320" width="220" height="150" as="geometry" />
        </mxCell>
        <mxCell id="customer-attrs" value="- name: string&#xA;- defaultAddress: Address&#xA;- loyaltyPoints: int" style="text;strokeColor=none;fillColor=none;align=left;verticalAlign=middle;spacingLeft=8;spacingRight=8;overflow=hidden;rotatable=0;points=[[0,0.5],[1,0.5]];portConstraint=eastwest;fontSize=12;" vertex="1" parent="customer">
          <mxGeometry y="30" width="220" height="55" as="geometry" />
        </mxCell>
        <mxCell id="customer-sep" value="" style="line;strokeWidth=1;fillColor=none;strokeColor=#6c8ebf;rotatable=0;points=[];portConstraint=eastwest;" vertex="1" parent="customer">
          <mxGeometry y="85" width="220" height="8" as="geometry" />
        </mxCell>
        <mxCell id="customer-methods" value="+ placeOrder(cart: Cart): Order&#xA;+ history(): Order[]&#xA;+ redeemPoints(n: int): void" style="text;strokeColor=none;fillColor=none;align=left;verticalAlign=middle;spacingLeft=8;spacingRight=8;overflow=hidden;rotatable=0;points=[[0,0.5],[1,0.5]];portConstraint=eastwest;fontSize=12;" vertex="1" parent="customer">
          <mxGeometry y="93" width="220" height="57" as="geometry" />
        </mxCell>

        <!-- ─── GuestAccount (extends Account) ─────────────────────────── -->
        <mxCell id="guest" value="&#129095; GuestAccount" style="swimlane;fontStyle=1;align=center;verticalAlign=top;childLayout=stackLayout;horizontal=1;startSize=30;horizontalStack=0;resizeParent=1;resizeParentMax=0;collapsible=0;marginBottom=0;swimlaneFillColor=#ffffff;fillColor=#dae8fc;strokeColor=#6c8ebf;fontSize=13;" vertex="1" parent="1">
          <mxGeometry x="320" y="320" width="220" height="120" as="geometry" />
        </mxCell>
        <mxCell id="guest-attrs" value="- sessionToken: string&#xA;- expiresAt: DateTime" style="text;strokeColor=none;fillColor=none;align=left;verticalAlign=middle;spacingLeft=8;spacingRight=8;overflow=hidden;fontSize=12;" vertex="1" parent="guest">
          <mxGeometry y="30" width="220" height="40" as="geometry" />
        </mxCell>
        <mxCell id="guest-sep" value="" style="line;strokeWidth=1;fillColor=none;strokeColor=#6c8ebf;rotatable=0;points=[];portConstraint=eastwest;" vertex="1" parent="guest">
          <mxGeometry y="70" width="220" height="8" as="geometry" />
        </mxCell>
        <mxCell id="guest-methods" value="+ promote(email: string): Customer&#xA;+ keepCart(): void" style="text;strokeColor=none;fillColor=none;align=left;verticalAlign=middle;spacingLeft=8;spacingRight=8;overflow=hidden;fontSize=12;" vertex="1" parent="guest">
          <mxGeometry y="78" width="220" height="42" as="geometry" />
        </mxCell>

        <!-- ─── Order ──────────────────────────────────────────────────── -->
        <mxCell id="order" value="&#129095; Order" style="swimlane;fontStyle=1;align=center;verticalAlign=top;childLayout=stackLayout;horizontal=1;startSize=30;horizontalStack=0;resizeParent=1;resizeParentMax=0;collapsible=0;marginBottom=0;swimlaneFillColor=#ffffff;fillColor=#dae8fc;strokeColor=#6c8ebf;fontSize=13;" vertex="1" parent="1">
          <mxGeometry x="600" y="320" width="240" height="180" as="geometry" />
        </mxCell>
        <mxCell id="order-attrs" value="- id: UUID&#xA;- customerId: UUID&#xA;- status: OrderStatus&#xA;- total: Money&#xA;- placedAt: DateTime" style="text;strokeColor=none;fillColor=none;align=left;verticalAlign=middle;spacingLeft=8;spacingRight=8;overflow=hidden;fontSize=12;" vertex="1" parent="order">
          <mxGeometry y="30" width="240" height="80" as="geometry" />
        </mxCell>
        <mxCell id="order-sep" value="" style="line;strokeWidth=1;fillColor=none;strokeColor=#6c8ebf;rotatable=0;points=[];portConstraint=eastwest;" vertex="1" parent="order">
          <mxGeometry y="110" width="240" height="8" as="geometry" />
        </mxCell>
        <mxCell id="order-methods" value="+ addLine(p: Product, qty: int): void&#xA;+ checkout(payment: Payment): Receipt&#xA;+ cancel(reason: string): void" style="text;strokeColor=none;fillColor=none;align=left;verticalAlign=middle;spacingLeft=8;spacingRight=8;overflow=hidden;fontSize=12;" vertex="1" parent="order">
          <mxGeometry y="118" width="240" height="62" as="geometry" />
        </mxCell>

        <!-- ─── OrderLine ──────────────────────────────────────────────── -->
        <mxCell id="orderline" value="&#129093; OrderLine" style="swimlane;fontStyle=1;align=center;verticalAlign=top;childLayout=stackLayout;horizontal=1;startSize=30;horizontalStack=0;resizeParent=1;resizeParentMax=0;collapsible=0;marginBottom=0;swimlaneFillColor=#ffffff;fillColor=#d5e8d4;strokeColor=#82b366;fontSize=13;" vertex="1" parent="1">
          <mxGeometry x="900" y="320" width="220" height="130" as="geometry" />
        </mxCell>
        <mxCell id="orderline-attrs" value="- productId: UUID&#xA;- quantity: int&#xA;- unitPrice: Money&#xA;- subtotal: Money" style="text;strokeColor=none;fillColor=none;align=left;verticalAlign=middle;spacingLeft=8;spacingRight=8;overflow=hidden;fontSize=12;" vertex="1" parent="orderline">
          <mxGeometry y="30" width="220" height="65" as="geometry" />
        </mxCell>
        <mxCell id="orderline-sep" value="" style="line;strokeWidth=1;fillColor=none;strokeColor=#82b366;rotatable=0;points=[];portConstraint=eastwest;" vertex="1" parent="orderline">
          <mxGeometry y="95" width="220" height="8" as="geometry" />
        </mxCell>
        <mxCell id="orderline-methods" value="+ recompute(): Money" style="text;strokeColor=none;fillColor=none;align=left;verticalAlign=middle;spacingLeft=8;spacingRight=8;overflow=hidden;fontSize=12;" vertex="1" parent="orderline">
          <mxGeometry y="103" width="220" height="27" as="geometry" />
        </mxCell>

        <!-- ─── Product ────────────────────────────────────────────────── -->
        <mxCell id="product" value="&#129095; Product" style="swimlane;fontStyle=1;align=center;verticalAlign=top;childLayout=stackLayout;horizontal=1;startSize=30;horizontalStack=0;resizeParent=1;resizeParentMax=0;collapsible=0;marginBottom=0;swimlaneFillColor=#ffffff;fillColor=#dae8fc;strokeColor=#6c8ebf;fontSize=13;" vertex="1" parent="1">
          <mxGeometry x="1180" y="320" width="200" height="130" as="geometry" />
        </mxCell>
        <mxCell id="product-attrs" value="- sku: string&#xA;- name: string&#xA;- price: Money&#xA;- stock: int" style="text;strokeColor=none;fillColor=none;align=left;verticalAlign=middle;spacingLeft=8;spacingRight=8;overflow=hidden;fontSize=12;" vertex="1" parent="product">
          <mxGeometry y="30" width="200" height="65" as="geometry" />
        </mxCell>
        <mxCell id="product-sep" value="" style="line;strokeWidth=1;fillColor=none;strokeColor=#6c8ebf;rotatable=0;points=[];portConstraint=eastwest;" vertex="1" parent="product">
          <mxGeometry y="95" width="200" height="8" as="geometry" />
        </mxCell>
        <mxCell id="product-methods" value="+ reserve(qty: int): bool" style="text;strokeColor=none;fillColor=none;align=left;verticalAlign=middle;spacingLeft=8;spacingRight=8;overflow=hidden;fontSize=12;" vertex="1" parent="product">
          <mxGeometry y="103" width="200" height="27" as="geometry" />
        </mxCell>

        <!-- ─── Payment (implements Payable) ───────────────────────────── -->
        <mxCell id="payment" value="&#129095; Payment" style="swimlane;fontStyle=1;align=center;verticalAlign=top;childLayout=stackLayout;horizontal=1;startSize=30;horizontalStack=0;resizeParent=1;resizeParentMax=0;collapsible=0;marginBottom=0;swimlaneFillColor=#ffffff;fillColor=#dae8fc;strokeColor=#6c8ebf;fontSize=13;" vertex="1" parent="1">
          <mxGeometry x="600" y="600" width="240" height="160" as="geometry" />
        </mxCell>
        <mxCell id="payment-attrs" value="- id: UUID&#xA;- method: PaymentMethod&#xA;- amount: Money&#xA;- status: PaymentStatus" style="text;strokeColor=none;fillColor=none;align=left;verticalAlign=middle;spacingLeft=8;spacingRight=8;overflow=hidden;fontSize=12;" vertex="1" parent="payment">
          <mxGeometry y="30" width="240" height="70" as="geometry" />
        </mxCell>
        <mxCell id="payment-sep" value="" style="line;strokeWidth=1;fillColor=none;strokeColor=#6c8ebf;rotatable=0;points=[];portConstraint=eastwest;" vertex="1" parent="payment">
          <mxGeometry y="100" width="240" height="8" as="geometry" />
        </mxCell>
        <mxCell id="payment-methods" value="+ process(): Receipt&#xA;+ refund(amount: Money): void" style="text;strokeColor=none;fillColor=none;align=left;verticalAlign=middle;spacingLeft=8;spacingRight=8;overflow=hidden;fontSize=12;" vertex="1" parent="payment">
          <mxGeometry y="108" width="240" height="52" as="geometry" />
        </mxCell>

        <!-- ─── Payable interface ──────────────────────────────────────── -->
        <mxCell id="payable" value="&lt;&lt;interface&gt;&gt; Payable" style="swimlane;fontStyle=2;align=center;verticalAlign=top;childLayout=stackLayout;horizontal=1;startSize=30;horizontalStack=0;resizeParent=1;resizeParentMax=0;collapsible=0;marginBottom=0;swimlaneFillColor=#ffffff;fillColor=#fff2cc;strokeColor=#d6b656;fontSize=13;" vertex="1" parent="1">
          <mxGeometry x="900" y="600" width="220" height="90" as="geometry" />
        </mxCell>
        <mxCell id="payable-methods" value="+ process(): Receipt&#xA;+ refund(amount: Money): void" style="text;strokeColor=none;fillColor=none;align=left;verticalAlign=middle;spacingLeft=8;spacingRight=8;overflow=hidden;fontSize=12;fontStyle=2;" vertex="1" parent="payable">
          <mxGeometry y="30" width="220" height="60" as="geometry" />
        </mxCell>

        <!-- ─── InventoryService ───────────────────────────────────────── -->
        <mxCell id="inventory" value="&#129082; InventoryService" style="swimlane;fontStyle=1;align=center;verticalAlign=top;childLayout=stackLayout;horizontal=1;startSize=30;horizontalStack=0;resizeParent=1;resizeParentMax=0;collapsible=0;marginBottom=0;swimlaneFillColor=#ffffff;fillColor=#ffe6cc;strokeColor=#d79b00;fontSize=13;" vertex="1" parent="1">
          <mxGeometry x="1180" y="600" width="200" height="120" as="geometry" />
        </mxCell>
        <mxCell id="inventory-methods" value="+ available(sku: string): int&#xA;+ reserve(sku, qty): Reservation&#xA;+ release(reservation): void" style="text;strokeColor=none;fillColor=none;align=left;verticalAlign=middle;spacingLeft=8;spacingRight=8;overflow=hidden;fontSize=12;" vertex="1" parent="inventory">
          <mxGeometry y="30" width="200" height="90" as="geometry" />
        </mxCell>

        <!-- ─── Relationships ──────────────────────────────────────────── -->

        <!-- Customer extends Account (inheritance: hollow triangle at parent) -->
        <mxCell id="rel-customer-account" edge="1" parent="1" source="customer" target="account"
          style="endArrow=block;endFill=0;endSize=14;html=1;rounded=0;exitX=0.5;exitY=0;exitDx=0;exitDy=0;entryX=0.5;entryY=1;entryDx=0;entryDy=0;strokeColor=#666666;">
          <mxGeometry relative="1" as="geometry" />
        </mxCell>
        <!-- GuestAccount extends Account -->
        <mxCell id="rel-guest-account" edge="1" parent="1" source="guest" target="account"
          style="endArrow=block;endFill=0;endSize=14;html=1;rounded=0;exitX=0.5;exitY=0;exitDx=0;exitDy=0;entryX=0.5;entryY=1;entryDx=0;entryDy=0;strokeColor=#666666;">
          <mxGeometry relative="1" as="geometry" />
        </mxCell>

        <!-- Order ◆—— OrderLine  (composition: filled diamond at Order side) -->
        <mxCell id="rel-order-orderline" edge="1" parent="1" source="order" target="orderline"
          style="startArrow=diamondThin;startFill=1;startSize=14;endArrow=none;html=1;rounded=0;exitX=1;exitY=0.5;exitDx=0;exitDy=0;entryX=0;entryY=0.5;entryDx=0;entryDy=0;strokeColor=#666666;"
          value="1 ◆—— 1..*">
          <mxGeometry relative="1" as="geometry">
            <Array as="points">
              <mxPoint x="870" y="410" />
            </Array>
          </mxGeometry>
        </mxCell>

        <!-- OrderLine ◇—— Product (aggregation: hollow diamond) -->
        <mxCell id="rel-orderline-product" edge="1" parent="1" source="orderline" target="product"
          style="startArrow=diamondThin;startFill=0;startSize=14;endArrow=none;html=1;rounded=0;exitX=1;exitY=0.5;exitDx=0;exitDy=0;entryX=0;entryY=0.5;entryDx=0;entryDy=0;strokeColor=#666666;"
          value="* ◇—— 1">
          <mxGeometry relative="1" as="geometry" />
        </mxCell>

        <!-- Order — Customer  (association) -->
        <mxCell id="rel-order-customer" edge="1" parent="1" source="order" target="customer"
          style="endArrow=open;endSize=10;html=1;rounded=0;exitX=0;exitY=0.5;exitDx=0;exitDy=0;entryX=1;entryY=0.5;entryDx=0;entryDy=0;strokeColor=#666666;"
          value="placed by &#xA;0..* → 1">
          <mxGeometry relative="1" as="geometry" />
        </mxCell>

        <!-- Order ◆—— Payment (composition) -->
        <mxCell id="rel-order-payment" edge="1" parent="1" source="order" target="payment"
          style="startArrow=diamondThin;startFill=1;startSize=14;endArrow=none;html=1;rounded=0;exitX=0.5;exitY=1;exitDx=0;exitDy=0;entryX=0.5;entryY=0;entryDx=0;entryDy=0;strokeColor=#666666;"
          value="1 ◆—— 0..1">
          <mxGeometry relative="1" as="geometry" />
        </mxCell>

        <!-- Payment ⊳⋯⋯ Payable (interface realization: dashed + hollow triangle) -->
        <mxCell id="rel-payment-payable" edge="1" parent="1" source="payment" target="payable"
          style="endArrow=block;endFill=0;endSize=14;dashed=1;html=1;rounded=0;exitX=1;exitY=0.5;exitDx=0;exitDy=0;entryX=0;entryY=0.5;entryDx=0;entryDy=0;strokeColor=#d6b656;"
          value="implements">
          <mxGeometry relative="1" as="geometry" />
        </mxCell>

        <!-- Order ⇢ InventoryService (dependency: dashed open arrow) -->
        <mxCell id="rel-order-inventory" edge="1" parent="1" source="order" target="inventory"
          style="endArrow=open;endSize=10;dashed=1;html=1;rounded=0;exitX=1;exitY=1;exitDx=0;exitDy=0;entryX=0;entryY=0;entryDx=0;entryDy=0;strokeColor=#d79b00;"
          value="reserves stock via">
          <mxGeometry relative="1" as="geometry" />
        </mxCell>

      </root>
    </mxGraphModel>
  </diagram>
</mxfile>
```

The result is a one-screen-tall diagram with: 1 title + 1 legend, 1 abstract
class, 1 interface, 5 entities, 1 value object, 1 service, 7 relationships
(2 inheritances, 2 compositions, 1 aggregation, 1 association, 1 interface
realization, 1 dependency), and a color-coded palette (blue entities, green
value object, yellow interface, orange service). Adapt every name +
attribute + method to the user's topic.

---

## Style reference — when adapting to other diagram types

### Class diagram cells

**Class container** (always swimlane with stackLayout):
```
swimlane;fontStyle=1;align=center;verticalAlign=top;childLayout=stackLayout;
horizontal=1;startSize=30;collapsible=0;marginBottom=0;swimlaneFillColor=#ffffff;
fillColor=<bg>;strokeColor=<border>;fontSize=13;
```

**Attribute/method compartment** (text child of swimlane):
```
text;strokeColor=none;fillColor=none;align=left;verticalAlign=middle;
spacingLeft=8;spacingRight=8;overflow=hidden;fontSize=12;
```

**Separator line** between attrs and methods (in swimlane):
```
line;strokeWidth=1;fillColor=none;strokeColor=<same as border>;
rotatable=0;points=[];portConstraint=eastwest;
```

### Relationship styles (edges)

| Relationship | Style fragment |
|---|---|
| **Inheritance** (`extends`) | `endArrow=block;endFill=0;endSize=14;` |
| **Interface realization** (`implements`) | `endArrow=block;endFill=0;endSize=14;dashed=1;` |
| **Composition** (◆—) | `startArrow=diamondThin;startFill=1;startSize=14;endArrow=none;` |
| **Aggregation** (◇—) | `startArrow=diamondThin;startFill=0;startSize=14;endArrow=none;` |
| **Association** | `endArrow=open;endSize=10;` |
| **Dependency** (⇢) | `endArrow=open;endSize=10;dashed=1;` |
| **Bidirectional** | `startArrow=open;endArrow=open;` |

Add `+ "html=1;rounded=0;"` to every edge. The `source` + `target` attributes
on the `<mxCell>` connect endpoints; `exitX/exitY` + `entryX/entryY` in the
style fine-tune where the line meets each side (0/0=top-left, 1/1=bottom-right,
0.5/0=top-center, etc.).

### Color palette (use consistently across one diagram)

| Category | Fill | Stroke | Use for |
|---|---|---|---|
| Blue | `#dae8fc` | `#6c8ebf` | Entities / aggregates |
| Green | `#d5e8d4` | `#82b366` | Value objects |
| Yellow | `#fff2cc` | `#d6b656` | Interfaces / contracts |
| Orange | `#ffe6cc` | `#d79b00` | Services / external |
| Red | `#f8cecc` | `#b85450` | Aggregates root / important |
| Gray | `#f5f5f5` | `#666666` | Notes / metadata |

### Other diagram types — quick reference

**Sequence diagram**: lifelines are tall narrow swimlanes; messages are horizontal
edges between them. Activation boxes go on the lifeline.

**ER diagram**: use `shape=table` + `swimlane` for tables with column rows. Edges
use `endArrow=ERmany;startArrow=ERone` style fragments for cardinality.

**C4 component diagram**: each component is a rounded rectangle with technology
in italics. Group with containers (`shape=mxgraph.c4.container`). Boundary lines
are dashed.

**State diagram**: states are rounded rectangles, initial state is a solid black
circle (`shape=startState`), final state is a circled black dot (`shape=endState`).
Transitions are open arrows with the event name as label.

---

## Step 3 — Confirm completion

Call `complete` with a summary like:
**"Generated a `<topic>` <diagram type> with N classes/entities and M relationships. The diagram renders inline in the new ktw-gui drawio tab; click the download icon in the iframe toolbar to save the `.drawio` file for editing in your local drawio app."**

## Notes for the LLM

- **The default template above is the QUALITY BAR**. Whatever topic the user
  picks, the output must be equally rich: legend at the top-right, color-coded
  shapes, multiple relationship types, multiplicity labels, stereotypes, real
  attribute/method names with type annotations.
- **Adapt `pageWidth` / `pageHeight`** based on the diagram's footprint
  (default 1400 × 1000 fits 6-8 classes in a 2-row grid).
- **Position carefully** — leave 40-60 px between adjacent classes; align
  rows on the same `y` coordinate. drawio doesn't auto-layout the embedded
  XML, so the LLM is responsible for the geometry.
- **Use HTML entities** for diamonds in labels (`◆` `◇`), arrows (`▷` `⇢`),
  and special chars. `&#xA;` is the line break inside `value` attrs.
- **No `persistent` flag** — this is a one-shot demo. The artifact auto-completes
  when the tool call ends.
- **Save button is host-side** — you don't need to call any save tool; the user
  clicks the download icon in the iframe and the file downloads via the browser.
