# Network & Infrastructure Diagrams

Syntax reference for BlockDiag family (BlockDiag, SeqDiag, ActDiag, NwDiag, RackDiag, PacketDiag).

---

## NwDiag — Network Topology

```nwdiag
nwdiag {
    internet [shape = cloud];
    internet -- router;

    network dmz {
        address = "210.x.x.x/24"
        router [address = "210.x.x.1"];
        web01 [address = "210.x.x.2"];
        web02 [address = "210.x.x.3"];
    }

    network internal {
        address = "172.x.x.x/24"
        router [address = "172.x.x.1"];
        app01 [address = "172.x.x.2"];
        db01 [address = "172.x.x.3"];
    }
}
```

### Multi-Network Host

```nwdiag
nwdiag {
    network frontend {
        address = "10.0.1.0/24"
        lb [address = "10.0.1.1", description = "Load Balancer"];
        web [address = "10.0.1.2"];
    }
    network backend {
        address = "10.0.2.0/24"
        web [address = "10.0.2.1"];
        api [address = "10.0.2.2"];
        db [address = "10.0.2.3"];
    }
}
```

---

## RackDiag — Server Rack Layouts

```rackdiag
rackdiag {
    16U;
    description = "Production Rack A";

    1: UPS [2U, color = "#ff9999"];
    3: Patch Panel [1U];
    4: Firewall [1U, color = "#ffcc99"];
    5: Switch [1U, color = "#99ccff"];
    6: Web Server 1 [1U];
    7: Web Server 2 [1U];
    8: App Server [2U, color = "#99ff99"];
    10: DB Server [2U, color = "#cc99ff"];
    12: Storage [4U, color = "#ffff99"];
}
```

---

## PacketDiag — Packet/Protocol Headers

```packetdiag
packetdiag {
    colwidth = 32;
    node_height = 72;

    0-3: Version;
    4-7: IHL;
    8-15: Type of Service;
    16-31: Total Length;
    32-47: Identification;
    48-50: Flags;
    51-63: Fragment Offset;
    64-71: TTL;
    72-79: Protocol;
    80-95: Header Checksum;
    96-127: Source Address;
    128-159: Destination Address;
}
```

---

## BlockDiag — Simple Block Diagrams

```blockdiag
blockdiag {
    orientation = portrait;

    A [label = "Input"];
    B [label = "Process", color = "#99ccff"];
    C [label = "Validate", shape = diamond];
    D [label = "Output", color = "#99ff99"];
    E [label = "Error", color = "#ff9999"];

    A -> B -> C;
    C -> D [label = "OK"];
    C -> E [label = "Fail"];
    E -> B [style = dashed];
}
```

Shapes: `box` (default), `roundedbox`, `diamond`, `ellipse`, `note`, `cloud`, `mail`, `textbox`, `dots`, `beginpoint`, `endpoint`, `minidiamond`, `actor`

Attributes: `label`, `color`, `style` (dashed, dotted), `textcolor`, `numbered`, `orientation` (portrait, landscape)

---

## SeqDiag — Sequence Diagrams

```seqdiag
seqdiag {
    browser -> webserver [label = "GET /index"];
    browser <-- webserver [label = "HTML"];
    browser -> webserver [label = "POST /api/data"];
    webserver -> app [label = "process()"];
    app -> database [label = "SELECT"];
    app <-- database [label = "results"];
    webserver <-- app [label = "JSON"];
    browser <-- webserver [label = "200 OK"];
}
```

Arrow types: `->` request, `<--` response, `-->` dashed, `=>` async

Features: `activation`, `autonumber`, `separator`, `note`

---

## ActDiag — Activity Diagrams

```actdiag
actdiag {
    write -> review -> approve -> publish;

    lane "Author" {
        write [label = "Write Draft"];
    }
    lane "Reviewer" {
        review [label = "Peer Review"];
    }
    lane "Manager" {
        approve [label = "Approve"];
    }
    lane "System" {
        publish [label = "Publish"];
    }
}
```

Features: `lane` for swimlanes, `color` attribute, `label` attribute
