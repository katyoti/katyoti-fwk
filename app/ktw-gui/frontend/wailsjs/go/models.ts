export namespace api {
	
	export class Actor {
	    id: string;
	    kind: string;
	
	    static createFrom(source: any = {}) {
	        return new Actor(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	    }
	}
	export class Badge {
	    id: string;
	    label: string;
	    icon?: string;
	    color?: string;
	    tooltip?: string;
	    capability?: string;
	
	    static createFrom(source: any = {}) {
	        return new Badge(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.icon = source["icon"];
	        this.color = source["color"];
	        this.tooltip = source["tooltip"];
	        this.capability = source["capability"];
	    }
	}
	export class DocHint {
	    level?: number;
	
	    static createFrom(source: any = {}) {
	        return new DocHint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.level = source["level"];
	    }
	}
	export class DeckHint {
	    layout?: string;
	    group?: string;
	
	    static createFrom(source: any = {}) {
	        return new DeckHint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.layout = source["layout"];
	        this.group = source["group"];
	    }
	}
	export class TemplateSource {
	    from: string;
	    value?: string;
	    note?: string;
	
	    static createFrom(source: any = {}) {
	        return new TemplateSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.from = source["from"];
	        this.value = source["value"];
	        this.note = source["note"];
	    }
	}
	export class TemplateSection {
	    id: string;
	    include?: string;
	    title?: string;
	    intent?: string;
	    sources?: TemplateSource[];
	    guidance?: string;
	    required?: boolean;
	    repeat?: string;
	    weight?: number;
	    deck?: DeckHint;
	    doc?: DocHint;
	
	    static createFrom(source: any = {}) {
	        return new TemplateSection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.include = source["include"];
	        this.title = source["title"];
	        this.intent = source["intent"];
	        this.sources = this.convertValues(source["sources"], TemplateSource);
	        this.guidance = source["guidance"];
	        this.required = source["required"];
	        this.repeat = source["repeat"];
	        this.weight = source["weight"];
	        this.deck = this.convertValues(source["deck"], DeckHint);
	        this.doc = this.convertValues(source["doc"], DocHint);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ContentTemplate {
	    name: string;
	    label?: string;
	    description: string;
	    purpose?: string;
	    target?: string;
	    partial?: boolean;
	    sections: TemplateSection[];
	
	    static createFrom(source: any = {}) {
	        return new ContentTemplate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.label = source["label"];
	        this.description = source["description"];
	        this.purpose = source["purpose"];
	        this.target = source["target"];
	        this.partial = source["partial"];
	        this.sections = this.convertValues(source["sections"], TemplateSection);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class FunctionAnswer {
	    function: string;
	    args?: any[];
	    answer?: any;
	    error?: string;
	    at_move?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FunctionAnswer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.function = source["function"];
	        this.args = source["args"];
	        this.answer = source["answer"];
	        this.error = source["error"];
	        this.at_move = source["at_move"];
	    }
	}
	export class LayoutSlot {
	    name: string;
	    description?: string;
	    required?: boolean;
	    minItems?: number;
	    maxItems?: number;
	
	    static createFrom(source: any = {}) {
	        return new LayoutSlot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.required = source["required"];
	        this.minItems = source["minItems"];
	        this.maxItems = source["maxItems"];
	    }
	}
	export class LayoutBody {
	    name: string;
	    description: string;
	    slots: LayoutSlot[];
	    example?: string;
	
	    static createFrom(source: any = {}) {
	        return new LayoutBody(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.slots = this.convertValues(source["slots"], LayoutSlot);
	        this.example = source["example"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class LifecycleEntry {
	    from?: string;
	    to: string;
	    transition?: string;
	    by: Actor;
	    at: string;
	    note?: string;
	    forced?: boolean;
	    calls?: FunctionAnswer[];
	
	    static createFrom(source: any = {}) {
	        return new LifecycleEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.from = source["from"];
	        this.to = source["to"];
	        this.transition = source["transition"];
	        this.by = this.convertValues(source["by"], Actor);
	        this.at = source["at"];
	        this.note = source["note"];
	        this.forced = source["forced"];
	        this.calls = this.convertValues(source["calls"], FunctionAnswer);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TransitionOption {
	    name: string;
	    to: string;
	    guard?: string;
	    guard_status: string;
	    calls?: FunctionAnswer[];
	    allowed: boolean;
	    reason?: string;
	    force?: string;
	
	    static createFrom(source: any = {}) {
	        return new TransitionOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.to = source["to"];
	        this.guard = source["guard"];
	        this.guard_status = source["guard_status"];
	        this.calls = this.convertValues(source["calls"], FunctionAnswer);
	        this.allowed = source["allowed"];
	        this.reason = source["reason"];
	        this.force = source["force"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class WorkflowState {
	    name: string;
	    description?: string;
	    terminal?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new WorkflowState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.terminal = source["terminal"];
	    }
	}
	export class LifecycleOptions {
	    capability?: string;
	    workflow: string;
	    workflow_label?: string;
	    state: string;
	    terminal?: boolean;
	    states: WorkflowState[];
	    history?: LifecycleEntry[];
	    state_colors?: Record<string, string>;
	    transitions: TransitionOption[];
	
	    static createFrom(source: any = {}) {
	        return new LifecycleOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.capability = source["capability"];
	        this.workflow = source["workflow"];
	        this.workflow_label = source["workflow_label"];
	        this.state = source["state"];
	        this.terminal = source["terminal"];
	        this.states = this.convertValues(source["states"], WorkflowState);
	        this.history = this.convertValues(source["history"], LifecycleEntry);
	        this.state_colors = source["state_colors"];
	        this.transitions = this.convertValues(source["transitions"], TransitionOption);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PanelRefresh {
	    scope?: string;
	    types?: string[];
	
	    static createFrom(source: any = {}) {
	        return new PanelRefresh(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scope = source["scope"];
	        this.types = source["types"];
	    }
	}
	export class TemplatePart {
	    section: TemplateSection;
	    groups?: string[];
	
	    static createFrom(source: any = {}) {
	        return new TemplatePart(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.section = this.convertValues(source["section"], TemplateSection);
	        this.groups = source["groups"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	

}

export namespace deckbody {
	
	export class ThemeColour {
	    name: string;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new ThemeColour(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.value = source["value"];
	    }
	}

}

export namespace main {
	
	export class CapCheckResult {
	    status: string;
	    desc: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new CapCheckResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.desc = source["desc"];
	        this.error = source["error"];
	    }
	}
	export class CleanAllSessionsResult {
	    stopped: number;
	    deleted: number;
	    failed: number;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new CleanAllSessionsResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.stopped = source["stopped"];
	        this.deleted = source["deleted"];
	        this.failed = source["failed"];
	        this.error = source["error"];
	    }
	}
	export class ContentTemplateObjectResult {
	    template: api.ContentTemplate;
	    parts: api.TemplatePart[];
	    expandError?: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ContentTemplateObjectResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.template = this.convertValues(source["template"], api.ContentTemplate);
	        this.parts = this.convertValues(source["parts"], api.TemplatePart);
	        this.expandError = source["expandError"];
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ContributedEngine {
	    id: string;
	    provider: string;
	    uri: string;
	    label?: string;
	    accepts: string[];
	    capabilities?: render.EngineCapabilities;
	    files?: render.EngineFile[];
	
	    static createFrom(source: any = {}) {
	        return new ContributedEngine(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.provider = source["provider"];
	        this.uri = source["uri"];
	        this.label = source["label"];
	        this.accepts = source["accepts"];
	        this.capabilities = this.convertValues(source["capabilities"], render.EngineCapabilities);
	        this.files = this.convertValues(source["files"], render.EngineFile);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ProviderView {
	    name: string;
	    type: string;
	    known: boolean;
	    declared: boolean;
	    mutable: boolean;
	    digest?: string;
	    drops: number;
	
	    static createFrom(source: any = {}) {
	        return new ProviderView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.known = source["known"];
	        this.declared = source["declared"];
	        this.mutable = source["mutable"];
	        this.digest = source["digest"];
	        this.drops = source["drops"];
	    }
	}
	export class LayerView {
	    name: string;
	    declared: boolean;
	    carried: boolean;
	    required?: boolean;
	    default?: boolean;
	    hash?: string;
	    size?: number;
	    band?: string;
	    mediaType?: string;
	    resident?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new LayerView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.declared = source["declared"];
	        this.carried = source["carried"];
	        this.required = source["required"];
	        this.default = source["default"];
	        this.hash = source["hash"];
	        this.size = source["size"];
	        this.band = source["band"];
	        this.mediaType = source["mediaType"];
	        this.resident = source["resident"];
	    }
	}
	export class KindView {
	    uri?: string;
	    name: string;
	    declared: boolean;
	    schema?: string;
	    validate?: string;
	    execution?: string;
	    defaultLayer?: string;
	    budget?: number;
	    findings?: string[];
	
	    static createFrom(source: any = {}) {
	        return new KindView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.uri = source["uri"];
	        this.name = source["name"];
	        this.declared = source["declared"];
	        this.schema = source["schema"];
	        this.validate = source["validate"];
	        this.execution = source["execution"];
	        this.defaultLayer = source["defaultLayer"];
	        this.budget = source["budget"];
	        this.findings = source["findings"];
	    }
	}
	export class ContributionDetail {
	    provider: string;
	    type: string;
	    id: string;
	    uri: string;
	    schema?: string;
	    name?: string;
	    description?: string;
	    category?: string;
	    iconTabler?: string;
	    iconEmoji?: string;
	    hashtags?: string[];
	    advertised: boolean;
	    annotations?: Record<string, string>;
	    validation: string;
	    validationErrors?: string[];
	    kind: KindView;
	    layers: LayerView[];
	    providerInfo: ProviderView;
	    hasBody: boolean;
	    isBinary: boolean;
	    bodySize: number;
	    mediaType?: string;
	    bodySource?: string;
	    body?: string;
	    refused?: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ContributionDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.type = source["type"];
	        this.id = source["id"];
	        this.uri = source["uri"];
	        this.schema = source["schema"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.category = source["category"];
	        this.iconTabler = source["iconTabler"];
	        this.iconEmoji = source["iconEmoji"];
	        this.hashtags = source["hashtags"];
	        this.advertised = source["advertised"];
	        this.annotations = source["annotations"];
	        this.validation = source["validation"];
	        this.validationErrors = source["validationErrors"];
	        this.kind = this.convertValues(source["kind"], KindView);
	        this.layers = this.convertValues(source["layers"], LayerView);
	        this.providerInfo = this.convertValues(source["providerInfo"], ProviderView);
	        this.hasBody = source["hasBody"];
	        this.isBinary = source["isBinary"];
	        this.bodySize = source["bodySize"];
	        this.mediaType = source["mediaType"];
	        this.bodySource = source["bodySource"];
	        this.body = source["body"];
	        this.refused = source["refused"];
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ContributionSummary {
	    provider: string;
	    type: string;
	    id: string;
	    uri: string;
	    schema?: string;
	    name?: string;
	    description?: string;
	
	    static createFrom(source: any = {}) {
	        return new ContributionSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.type = source["type"];
	        this.id = source["id"];
	        this.uri = source["uri"];
	        this.schema = source["schema"];
	        this.name = source["name"];
	        this.description = source["description"];
	    }
	}
	export class DataTypeSummary {
	    provider: string;
	    id: string;
	    uri: string;
	    format?: string;
	    source?: string;
	
	    static createFrom(source: any = {}) {
	        return new DataTypeSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.id = source["id"];
	        this.uri = source["uri"];
	        this.format = source["format"];
	        this.source = source["source"];
	    }
	}
	export class DeckAssetsResult {
	    scope?: string;
	    version?: string;
	    missing?: string[];
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new DeckAssetsResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scope = source["scope"];
	        this.version = source["version"];
	        this.missing = source["missing"];
	        this.error = source["error"];
	    }
	}
	export class DeckEditorField {
	    name: string;
	    description?: string;
	    enum?: string[];
	
	    static createFrom(source: any = {}) {
	        return new DeckEditorField(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.enum = source["enum"];
	    }
	}
	export class DeckEditorTheme {
	    name: string;
	    description?: string;
	    variants?: Record<string, Array<string>>;
	    palette?: deckbody.ThemeColour[];
	
	    static createFrom(source: any = {}) {
	        return new DeckEditorTheme(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.variants = source["variants"];
	        this.palette = this.convertValues(source["palette"], deckbody.ThemeColour);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DeckEditorGrammarResult {
	    deck: DeckEditorField[];
	    slide: DeckEditorField[];
	    object: DeckEditorField[];
	    layouts: api.LayoutBody[];
	    themes: DeckEditorTheme[];
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new DeckEditorGrammarResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.deck = this.convertValues(source["deck"], DeckEditorField);
	        this.slide = this.convertValues(source["slide"], DeckEditorField);
	        this.object = this.convertValues(source["object"], DeckEditorField);
	        this.layouts = this.convertValues(source["layouts"], api.LayoutBody);
	        this.themes = this.convertValues(source["themes"], DeckEditorTheme);
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class DeckSlideGroupInfo {
	    name: string;
	    description: string;
	    provider: string;
	    slides: string;
	
	    static createFrom(source: any = {}) {
	        return new DeckSlideGroupInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.provider = source["provider"];
	        this.slides = source["slides"];
	    }
	}
	export class DeckThemeObjectResult {
	    name: string;
	    theme: string;
	    sample: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new DeckThemeObjectResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.theme = source["theme"];
	        this.sample = source["sample"];
	        this.error = source["error"];
	    }
	}
	export class DeleteSessionResult {
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new DeleteSessionResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.error = source["error"];
	    }
	}
	export class DisplayEngineWidgetResult {
	    html: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new DisplayEngineWidgetResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.html = source["html"];
	        this.error = source["error"];
	    }
	}
	export class DisplayEnginesResult {
	    engines: ContributedEngine[];
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new DisplayEnginesResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.engines = this.convertValues(source["engines"], ContributedEngine);
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DisplaySwitchState {
	    increment: string;
	    variable: string;
	    on: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DisplaySwitchState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.increment = source["increment"];
	        this.variable = source["variable"];
	        this.on = source["on"];
	    }
	}
	export class DocumentExportRunResult {
	    exportId?: string;
	    taskId?: string;
	    panel?: string;
	    document?: render.Document;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new DocumentExportRunResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.exportId = source["exportId"];
	        this.taskId = source["taskId"];
	        this.panel = source["panel"];
	        this.document = this.convertValues(source["document"], render.Document);
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DocumentOpenResult {
	    document: render.Document;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new DocumentOpenResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.document = this.convertValues(source["document"], render.Document);
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FeedbackCaptureResult {
	    png?: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new FeedbackCaptureResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.png = source["png"];
	        this.error = source["error"];
	    }
	}
	export class FeedbackConfigResult {
	    folder: string;
	    error?: string;
	    errorCode?: string;
	
	    static createFrom(source: any = {}) {
	        return new FeedbackConfigResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.folder = source["folder"];
	        this.error = source["error"];
	        this.errorCode = source["errorCode"];
	    }
	}
	export class FeedbackSubmitInput {
	    kind: string;
	    title: string;
	    description: string;
	    universe: string;
	    route: string;
	    screenshotPng: string;
	
	    static createFrom(source: any = {}) {
	        return new FeedbackSubmitInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.title = source["title"];
	        this.description = source["description"];
	        this.universe = source["universe"];
	        this.route = source["route"];
	        this.screenshotPng = source["screenshotPng"];
	    }
	}
	export class FeedbackSubmitResult {
	    name?: string;
	    path?: string;
	    error?: string;
	    errorCode?: string;
	
	    static createFrom(source: any = {}) {
	        return new FeedbackSubmitResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.error = source["error"];
	        this.errorCode = source["errorCode"];
	    }
	}
	export class SessionJournalToolCall {
	    id?: string;
	    name: string;
	    label?: string;
	    icon?: string;
	    input?: number[];
	    result?: string;
	    error?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SessionJournalToolCall(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.label = source["label"];
	        this.icon = source["icon"];
	        this.input = source["input"];
	        this.result = source["result"];
	        this.error = source["error"];
	    }
	}
	export class SessionJournalTurn {
	    number: number;
	    runId?: string;
	    runLabel?: string;
	    userText?: string;
	    assistText?: string;
	    toolCalls?: SessionJournalToolCall[];
	    durationMs?: number;
	    inputTok?: number;
	    outputTok?: number;
	
	    static createFrom(source: any = {}) {
	        return new SessionJournalTurn(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.number = source["number"];
	        this.runId = source["runId"];
	        this.runLabel = source["runLabel"];
	        this.userText = source["userText"];
	        this.assistText = source["assistText"];
	        this.toolCalls = this.convertValues(source["toolCalls"], SessionJournalToolCall);
	        this.durationMs = source["durationMs"];
	        this.inputTok = source["inputTok"];
	        this.outputTok = source["outputTok"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SessionModelCost {
	    model: string;
	    provider?: string;
	    runs: number;
	    inputTokens: number;
	    outputTokens: number;
	    cacheReadTokens?: number;
	    cacheWriteTokens?: number;
	    inputUsd?: number;
	    outputUsd?: number;
	    cacheReadUsd?: number;
	    cacheWriteUsd?: number;
	    totalUsd: number;
	
	    static createFrom(source: any = {}) {
	        return new SessionModelCost(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model = source["model"];
	        this.provider = source["provider"];
	        this.runs = source["runs"];
	        this.inputTokens = source["inputTokens"];
	        this.outputTokens = source["outputTokens"];
	        this.cacheReadTokens = source["cacheReadTokens"];
	        this.cacheWriteTokens = source["cacheWriteTokens"];
	        this.inputUsd = source["inputUsd"];
	        this.outputUsd = source["outputUsd"];
	        this.cacheReadUsd = source["cacheReadUsd"];
	        this.cacheWriteUsd = source["cacheWriteUsd"];
	        this.totalUsd = source["totalUsd"];
	    }
	}
	export class SessionJournalRun {
	    label: string;
	    runId: string;
	    parentRunId?: string;
	    task?: string;
	    model?: string;
	    provider?: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionJournalRun(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.runId = source["runId"];
	        this.parentRunId = source["parentRunId"];
	        this.task = source["task"];
	        this.model = source["model"];
	        this.provider = source["provider"];
	    }
	}
	export class SessionJournalConversation {
	    sessionId: string;
	    task?: string;
	    model?: string;
	    provider?: string;
	    systemMessage?: string;
	    toolNames?: string[];
	    runs?: SessionJournalRun[];
	    models?: SessionModelCost[];
	    turns?: SessionJournalTurn[];
	    totalTok?: number;
	    totalUsd?: number;
	    durationMs?: number;
	
	    static createFrom(source: any = {}) {
	        return new SessionJournalConversation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.task = source["task"];
	        this.model = source["model"];
	        this.provider = source["provider"];
	        this.systemMessage = source["systemMessage"];
	        this.toolNames = source["toolNames"];
	        this.runs = this.convertValues(source["runs"], SessionJournalRun);
	        this.models = this.convertValues(source["models"], SessionModelCost);
	        this.turns = this.convertValues(source["turns"], SessionJournalTurn);
	        this.totalTok = source["totalTok"];
	        this.totalUsd = source["totalUsd"];
	        this.durationMs = source["durationMs"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class GetSessionJournalResult {
	    conversation?: SessionJournalConversation;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new GetSessionJournalResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.conversation = this.convertValues(source["conversation"], SessionJournalConversation);
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SessionModelCallSSE {
	    type: string;
	    ts?: string;
	    data?: number[];
	
	    static createFrom(source: any = {}) {
	        return new SessionModelCallSSE(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.ts = source["ts"];
	        this.data = source["data"];
	    }
	}
	export class SessionModelCall {
	    callId: string;
	    provider?: string;
	    model?: string;
	    method?: string;
	    url?: string;
	    ts?: string;
	    status?: number;
	    durationMs?: number;
	    firstByteMs?: number;
	    error?: string;
	    inFlight?: boolean;
	    requestHeaders?: Record<string, string>;
	    requestBody?: number[];
	    responseHeaders?: Record<string, string>;
	    responseBody?: number[];
	    sseEvents?: SessionModelCallSSE[];
	    sseSummary?: number[];
	
	    static createFrom(source: any = {}) {
	        return new SessionModelCall(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.callId = source["callId"];
	        this.provider = source["provider"];
	        this.model = source["model"];
	        this.method = source["method"];
	        this.url = source["url"];
	        this.ts = source["ts"];
	        this.status = source["status"];
	        this.durationMs = source["durationMs"];
	        this.firstByteMs = source["firstByteMs"];
	        this.error = source["error"];
	        this.inFlight = source["inFlight"];
	        this.requestHeaders = source["requestHeaders"];
	        this.requestBody = source["requestBody"];
	        this.responseHeaders = source["responseHeaders"];
	        this.responseBody = source["responseBody"];
	        this.sseEvents = this.convertValues(source["sseEvents"], SessionModelCallSSE);
	        this.sseSummary = source["sseSummary"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class GetSessionModelCallsResult {
	    calls?: SessionModelCall[];
	    loggingOff?: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new GetSessionModelCallsResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.calls = this.convertValues(source["calls"], SessionModelCall);
	        this.loggingOff = source["loggingOff"];
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SessionTranscriptEntry {
	    type: string;
	    turn?: number;
	    timestamp: string;
	    data?: any;
	
	    static createFrom(source: any = {}) {
	        return new SessionTranscriptEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.turn = source["turn"];
	        this.timestamp = source["timestamp"];
	        this.data = source["data"];
	    }
	}
	export class SessionDetail {
	    id: string;
	    task: string;
	    date: string;
	    time: string;
	    status?: string;
	    turns?: number;
	    tokens?: number;
	    model?: string;
	    provider?: string;
	    durationMs?: number;
	    transcript?: SessionTranscriptEntry[];
	
	    static createFrom(source: any = {}) {
	        return new SessionDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.task = source["task"];
	        this.date = source["date"];
	        this.time = source["time"];
	        this.status = source["status"];
	        this.turns = source["turns"];
	        this.tokens = source["tokens"];
	        this.model = source["model"];
	        this.provider = source["provider"];
	        this.durationMs = source["durationMs"];
	        this.transcript = this.convertValues(source["transcript"], SessionTranscriptEntry);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class GetSessionResult {
	    detail?: SessionDetail;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new GetSessionResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.detail = this.convertValues(source["detail"], SessionDetail);
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SessionLedgerEntry {
	    kind: string;
	    state?: Record<string, any>;
	    version?: number;
	    reason?: string;
	    runId?: string;
	    ts?: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionLedgerEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.state = source["state"];
	        this.version = source["version"];
	        this.reason = source["reason"];
	        this.runId = source["runId"];
	        this.ts = source["ts"];
	    }
	}
	export class SessionCall {
	    node: string;
	    uri: string;
	    durationMs: number;
	    attempt: number;
	    outcome: string;
	    error?: string;
	    runId?: string;
	    ts?: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionCall(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.node = source["node"];
	        this.uri = source["uri"];
	        this.durationMs = source["durationMs"];
	        this.attempt = source["attempt"];
	        this.outcome = source["outcome"];
	        this.error = source["error"];
	        this.runId = source["runId"];
	        this.ts = source["ts"];
	    }
	}
	export class SessionMessage {
	    from: string;
	    to: string;
	    team?: string;
	    text: string;
	    summary?: string;
	    ts?: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.from = source["from"];
	        this.to = source["to"];
	        this.team = source["team"];
	        this.text = source["text"];
	        this.summary = source["summary"];
	        this.ts = source["ts"];
	    }
	}
	export class SessionTeamEntry {
	    action: string;
	    teamId?: string;
	    teamName?: string;
	    state?: string;
	    memberId?: string;
	    runId?: string;
	    detail?: Record<string, any>;
	    ts?: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionTeamEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.action = source["action"];
	        this.teamId = source["teamId"];
	        this.teamName = source["teamName"];
	        this.state = source["state"];
	        this.memberId = source["memberId"];
	        this.runId = source["runId"];
	        this.detail = source["detail"];
	        this.ts = source["ts"];
	    }
	}
	export class SessionTaskNode {
	    runId: string;
	    parentRunId?: string;
	    name: string;
	    kind?: string;
	    mode?: string;
	    commsMode?: string;
	    status?: string;
	    input?: Record<string, any>;
	    output?: Record<string, any>;
	    ts?: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionTaskNode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.runId = source["runId"];
	        this.parentRunId = source["parentRunId"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.mode = source["mode"];
	        this.commsMode = source["commsMode"];
	        this.status = source["status"];
	        this.input = source["input"];
	        this.output = source["output"];
	        this.ts = source["ts"];
	    }
	}
	export class SessionTaskTree {
	    sessionId: string;
	    model?: string;
	    totalTokens?: number;
	    durationMs?: number;
	    tasks?: SessionTaskNode[];
	    teamEvents?: SessionTeamEntry[];
	    messages?: SessionMessage[];
	    calls?: SessionCall[];
	    ledger?: SessionLedgerEntry[];
	
	    static createFrom(source: any = {}) {
	        return new SessionTaskTree(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.model = source["model"];
	        this.totalTokens = source["totalTokens"];
	        this.durationMs = source["durationMs"];
	        this.tasks = this.convertValues(source["tasks"], SessionTaskNode);
	        this.teamEvents = this.convertValues(source["teamEvents"], SessionTeamEntry);
	        this.messages = this.convertValues(source["messages"], SessionMessage);
	        this.calls = this.convertValues(source["calls"], SessionCall);
	        this.ledger = this.convertValues(source["ledger"], SessionLedgerEntry);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class GetSessionTaskTreeResult {
	    tree?: SessionTaskTree;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new GetSessionTaskTreeResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tree = this.convertValues(source["tree"], SessionTaskTree);
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class JSONSchemaResult {
	    schema?: number[];
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new JSONSchemaResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.schema = source["schema"];
	        this.error = source["error"];
	    }
	}
	export class KTSPCallResult {
	    response?: number[];
	    error?: string;
	    errorMsg?: usermsg.Msg;
	
	    static createFrom(source: any = {}) {
	        return new KTSPCallResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.response = source["response"];
	        this.error = source["error"];
	        this.errorMsg = this.convertValues(source["errorMsg"], usermsg.Msg);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class KTSPMethodSummary {
	    name: string;
	    description?: string;
	    inputSchema?: number[];
	    outputSchema?: number[];
	    tags?: string[];
	
	    static createFrom(source: any = {}) {
	        return new KTSPMethodSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.inputSchema = source["inputSchema"];
	        this.outputSchema = source["outputSchema"];
	        this.tags = source["tags"];
	    }
	}
	export class KTSPSlotSummary {
	    name: string;
	    type: string;
	    description?: string;
	    methods: KTSPMethodSummary[];
	
	    static createFrom(source: any = {}) {
	        return new KTSPSlotSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.description = source["description"];
	        this.methods = this.convertValues(source["methods"], KTSPMethodSummary);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class KTSPManifestSummary {
	    provider: string;
	    version?: string;
	    description?: string;
	    slots: KTSPSlotSummary[];
	
	    static createFrom(source: any = {}) {
	        return new KTSPManifestSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.version = source["version"];
	        this.description = source["description"];
	        this.slots = this.convertValues(source["slots"], KTSPSlotSummary);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	export class LayerContent {
	    name: string;
	    hash?: string;
	    size: number;
	    band?: string;
	    mediaType?: string;
	    refused?: string;
	    isBinary: boolean;
	    content?: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new LayerContent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.hash = source["hash"];
	        this.size = source["size"];
	        this.band = source["band"];
	        this.mediaType = source["mediaType"];
	        this.refused = source["refused"];
	        this.isBinary = source["isBinary"];
	        this.content = source["content"];
	        this.error = source["error"];
	    }
	}
	
	export class ListContributionsResult {
	    contributions: ContributionSummary[];
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ListContributionsResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.contributions = this.convertValues(source["contributions"], ContributionSummary);
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ListDataTypesResult {
	    dataTypes: DataTypeSummary[];
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ListDataTypesResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dataTypes = this.convertValues(source["dataTypes"], DataTypeSummary);
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ListKTSPManifestsResult {
	    manifests: KTSPManifestSummary[];
	    unavailable?: string[];
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ListKTSPManifestsResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.manifests = this.convertValues(source["manifests"], KTSPManifestSummary);
	        this.unavailable = source["unavailable"];
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RunningSession {
	    id: string;
	    parentSessionId?: string;
	    name: string;
	    status: string;
	    taskId: string;
	    parentTaskId?: string;
	    messageCount: number;
	    startedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new RunningSession(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.parentSessionId = source["parentSessionId"];
	        this.name = source["name"];
	        this.status = source["status"];
	        this.taskId = source["taskId"];
	        this.parentTaskId = source["parentTaskId"];
	        this.messageCount = source["messageCount"];
	        this.startedAt = source["startedAt"];
	    }
	}
	export class ListRunningSessionsResult {
	    sessions: RunningSession[];
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ListRunningSessionsResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessions = this.convertValues(source["sessions"], RunningSession);
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SessionInfo {
	    id: string;
	    task: string;
	    date: string;
	    time: string;
	    status?: string;
	    turns?: number;
	    tokens?: number;
	    model?: string;
	    durationMs?: number;
	
	    static createFrom(source: any = {}) {
	        return new SessionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.task = source["task"];
	        this.date = source["date"];
	        this.time = source["time"];
	        this.status = source["status"];
	        this.turns = source["turns"];
	        this.tokens = source["tokens"];
	        this.model = source["model"];
	        this.durationMs = source["durationMs"];
	    }
	}
	export class ListSessionsResult {
	    sessions?: SessionInfo[];
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ListSessionsResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessions = this.convertValues(source["sessions"], SessionInfo);
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ObjectVerifyRunResult {
	    taskId?: string;
	    panel?: string;
	    check?: CapCheckResult;
	
	    static createFrom(source: any = {}) {
	        return new ObjectVerifyRunResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.taskId = source["taskId"];
	        this.panel = source["panel"];
	        this.check = this.convertValues(source["check"], CapCheckResult);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PickFolderResult {
	    path?: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new PickFolderResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.error = source["error"];
	    }
	}
	
	export class RelaunchResult {
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new RelaunchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.error = source["error"];
	    }
	}
	export class ResumeSessionResult {
	    runID?: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ResumeSessionResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.runID = source["runID"];
	        this.error = source["error"];
	    }
	}
	
	export class SchemaASTResult {
	    ast?: number[];
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new SchemaASTResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ast = source["ast"];
	        this.error = source["error"];
	    }
	}
	export class SchemaViolation {
	    severity: string;
	    code?: string;
	    message: string;
	    path?: string;
	
	    static createFrom(source: any = {}) {
	        return new SchemaViolation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.severity = source["severity"];
	        this.code = source["code"];
	        this.message = source["message"];
	        this.path = source["path"];
	    }
	}
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	export class SkillInfo {
	    name: string;
	    provider: string;
	    description: string;
	    icon?: string;
	    iconName?: string;
	    hashtags?: string[];
	    category?: string;
	
	    static createFrom(source: any = {}) {
	        return new SkillInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.provider = source["provider"];
	        this.description = source["description"];
	        this.icon = source["icon"];
	        this.iconName = source["iconName"];
	        this.hashtags = source["hashtags"];
	        this.category = source["category"];
	    }
	}
	export class TaskInfo {
	    name: string;
	    title?: string;
	    provider?: string;
	    uri?: string;
	    capability: string;
	    description: string;
	    body?: string;
	    type: string;
	    icon?: string;
	    iconName?: string;
	    hashtags?: string[];
	    category?: string;
	    tags?: string[];
	    kind?: string;
	    types?: string[];
	    audience?: string;
	    inputSchemaURI?: string;
	    outputSchemaURI?: string;
	
	    static createFrom(source: any = {}) {
	        return new TaskInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.title = source["title"];
	        this.provider = source["provider"];
	        this.uri = source["uri"];
	        this.capability = source["capability"];
	        this.description = source["description"];
	        this.body = source["body"];
	        this.type = source["type"];
	        this.icon = source["icon"];
	        this.iconName = source["iconName"];
	        this.hashtags = source["hashtags"];
	        this.category = source["category"];
	        this.tags = source["tags"];
	        this.kind = source["kind"];
	        this.types = source["types"];
	        this.audience = source["audience"];
	        this.inputSchemaURI = source["inputSchemaURI"];
	        this.outputSchemaURI = source["outputSchemaURI"];
	    }
	}
	export class TerminalInfo {
	    id: string;
	    label: string;
	    icon: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new TerminalInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.icon = source["icon"];
	        this.path = source["path"];
	    }
	}
	export class UniverseActivateResult {
	    name?: string;
	    running: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new UniverseActivateResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.running = source["running"];
	        this.error = source["error"];
	    }
	}
	export class UniverseDeactivateResult {
	    name?: string;
	    running: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new UniverseDeactivateResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.running = source["running"];
	        this.error = source["error"];
	    }
	}
	export class UniverseListActiveResult {
	    active?: supervisor.ActiveUniverseInfo[];
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new UniverseListActiveResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.active = this.convertValues(source["active"], supervisor.ActiveUniverseInfo);
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ValidateSchemaResult {
	    valid: boolean;
	    violations?: SchemaViolation[];
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ValidateSchemaResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.valid = source["valid"];
	        this.violations = this.convertValues(source["violations"], SchemaViolation);
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class WorkspaceActionTaskResult {
	    task?: string;
	    title?: string;
	    description?: string;
	    inputSchemaURI?: string;
	    tags?: string[];
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceActionTaskResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.task = source["task"];
	        this.title = source["title"];
	        this.description = source["description"];
	        this.inputSchemaURI = source["inputSchemaURI"];
	        this.tags = source["tags"];
	        this.error = source["error"];
	    }
	}
	export class WorkspaceBadge {
	    id: string;
	    label: string;
	    icon?: string;
	    color?: string;
	    tooltip?: string;
	    capability?: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceBadge(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.icon = source["icon"];
	        this.color = source["color"];
	        this.tooltip = source["tooltip"];
	        this.capability = source["capability"];
	    }
	}
	export class WorkspaceCallMethodResult {
	    result?: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceCallMethodResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.result = source["result"];
	        this.error = source["error"];
	    }
	}
	export class WorkspaceCapabilityInfo {
	    id: string;
	    label?: string;
	    description?: string;
	    icon?: string;
	    source?: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceCapabilityInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.description = source["description"];
	        this.icon = source["icon"];
	        this.source = source["source"];
	    }
	}
	export class WorkspaceCapabilityPanelResult {
	    uri?: string;
	    descriptor?: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceCapabilityPanelResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.uri = source["uri"];
	        this.descriptor = source["descriptor"];
	        this.error = source["error"];
	    }
	}
	export class WorkspaceDescribeActionResult {
	    uri: string;
	    label?: string;
	    description?: string;
	    details?: string;
	    inputSchema?: string;
	    isAsync: boolean;
	    error?: string;
	    errorMsg?: usermsg.Msg;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceDescribeActionResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.uri = source["uri"];
	        this.label = source["label"];
	        this.description = source["description"];
	        this.details = source["details"];
	        this.inputSchema = source["inputSchema"];
	        this.isAsync = source["isAsync"];
	        this.error = source["error"];
	        this.errorMsg = this.convertValues(source["errorMsg"], usermsg.Msg);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class WorkspaceLinkKind {
	    name: string;
	    description?: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceLinkKind(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	    }
	}
	export class WorkspaceNamedFile {
	    capability: string;
	    name: string;
	    source: string;
	    path: string;
	    media?: string;
	    exists: boolean;
	    content?: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceNamedFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.capability = source["capability"];
	        this.name = source["name"];
	        this.source = source["source"];
	        this.path = source["path"];
	        this.media = source["media"];
	        this.exists = source["exists"];
	        this.content = source["content"];
	        this.error = source["error"];
	    }
	}
	export class WorkspaceNodeDiagnostic {
	    code: string;
	    label: string;
	    description?: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceNodeDiagnostic(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.code = source["code"];
	        this.label = source["label"];
	        this.description = source["description"];
	    }
	}
	export class WorkspaceNodeTypeChoice {
	    name: string;
	    label?: string;
	    icon?: string;
	    description?: string;
	    createTask?: string;
	    named?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceNodeTypeChoice(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.label = source["label"];
	        this.icon = source["icon"];
	        this.description = source["description"];
	        this.createTask = source["createTask"];
	        this.named = source["named"];
	    }
	}
	export class WorkspaceObject {
	    id: string;
	    type: string;
	    title: string;
	    module?: string;
	    depth: number;
	    hasChildren: boolean;
	    hasErrors?: boolean;
	    system?: boolean;
	    level?: string;
	    project?: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceObject(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.title = source["title"];
	        this.module = source["module"];
	        this.depth = source["depth"];
	        this.hasChildren = source["hasChildren"];
	        this.hasErrors = source["hasErrors"];
	        this.system = source["system"];
	        this.level = source["level"];
	        this.project = source["project"];
	    }
	}
	export class WorkspaceObjectAspect {
	    name: string;
	    label?: string;
	    description?: string;
	    capabilities?: string[];
	    declared: boolean;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceObjectAspect(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.label = source["label"];
	        this.description = source["description"];
	        this.capabilities = source["capabilities"];
	        this.declared = source["declared"];
	    }
	}
	export class WorkspaceObjectBody {
	    label: string;
	    description?: string;
	    format: string;
	    panel?: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceObjectBody(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.description = source["description"];
	        this.format = source["format"];
	        this.panel = source["panel"];
	    }
	}
	export class WorkspaceObjectChildRow {
	    id: string;
	    state?: WorkspaceBadge;
	    badges?: WorkspaceBadge[];
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceObjectChildRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.state = this.convertValues(source["state"], WorkspaceBadge);
	        this.badges = this.convertValues(source["badges"], WorkspaceBadge);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class WorkspaceObjectChildGroup {
	    type: string;
	    label?: string;
	    icon?: string;
	    count: number;
	    drawn?: boolean;
	    objects: WorkspaceObject[];
	    rows?: WorkspaceObjectChildRow[];
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceObjectChildGroup(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.label = source["label"];
	        this.icon = source["icon"];
	        this.count = source["count"];
	        this.drawn = source["drawn"];
	        this.objects = this.convertValues(source["objects"], WorkspaceObject);
	        this.rows = this.convertValues(source["rows"], WorkspaceObjectChildRow);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class WorkspaceObjectComment {
	    id: string;
	    by: string;
	    byKind?: string;
	    at: string;
	    text: string;
	    replyTo?: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceObjectComment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.by = source["by"];
	        this.byKind = source["byKind"];
	        this.at = source["at"];
	        this.text = source["text"];
	        this.replyTo = source["replyTo"];
	    }
	}
	export class WorkspaceObjectComments {
	    comments: WorkspaceObjectComment[];
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceObjectComments(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.comments = this.convertValues(source["comments"], WorkspaceObjectComment);
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class WorkspaceObjectLink {
	    kind: string;
	    label: string;
	    direction: string;
	    targetId: string;
	    targetType?: string;
	    targetTitle?: string;
	    resolved: boolean;
	    ended?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceObjectLink(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.label = source["label"];
	        this.direction = source["direction"];
	        this.targetId = source["targetId"];
	        this.targetType = source["targetType"];
	        this.targetTitle = source["targetTitle"];
	        this.resolved = source["resolved"];
	        this.ended = source["ended"];
	    }
	}
	export class WorkspaceQuickAction {
	    uri: string;
	    label: string;
	    description?: string;
	    icon?: string;
	    kind: string;
	    source: string;
	    confirm?: boolean;
	    preview?: boolean;
	    quick?: boolean;
	    disabled?: boolean;
	    disabledReason?: string;
	    boundInput?: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceQuickAction(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.uri = source["uri"];
	        this.label = source["label"];
	        this.description = source["description"];
	        this.icon = source["icon"];
	        this.kind = source["kind"];
	        this.source = source["source"];
	        this.confirm = source["confirm"];
	        this.preview = source["preview"];
	        this.quick = source["quick"];
	        this.disabled = source["disabled"];
	        this.disabledReason = source["disabledReason"];
	        this.boundInput = source["boundInput"];
	    }
	}
	export class WorkspacePolicy {
	    name: string;
	    label: string;
	    description?: string;
	    icon?: string;
	    hasCheck?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new WorkspacePolicy(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.label = source["label"];
	        this.description = source["description"];
	        this.icon = source["icon"];
	        this.hasCheck = source["hasCheck"];
	    }
	}
	export class WorkspaceObjectTab {
	    id: string;
	    name: string;
	    description?: string;
	    icon?: string;
	    emoji?: string;
	    order: number;
	    kind: string;
	    childType?: string;
	    content?: render.Def;
	    source?: string;
	    schema?: string;
	    refresh?: api.PanelRefresh;
	    input?: Record<string, any>;
	    capabilities?: string[];
	    verify?: boolean;
	    origin?: string;
	    section?: boolean;
	    header?: boolean;
	    more?: boolean;
	    body?: WorkspaceObjectBody;
	    badge?: api.Badge;
	    count?: string;
	    values?: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceObjectTab(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.icon = source["icon"];
	        this.emoji = source["emoji"];
	        this.order = source["order"];
	        this.kind = source["kind"];
	        this.childType = source["childType"];
	        this.content = this.convertValues(source["content"], render.Def);
	        this.source = source["source"];
	        this.schema = source["schema"];
	        this.refresh = this.convertValues(source["refresh"], api.PanelRefresh);
	        this.input = source["input"];
	        this.capabilities = source["capabilities"];
	        this.verify = source["verify"];
	        this.origin = source["origin"];
	        this.section = source["section"];
	        this.header = source["header"];
	        this.more = source["more"];
	        this.body = this.convertValues(source["body"], WorkspaceObjectBody);
	        this.badge = this.convertValues(source["badge"], api.Badge);
	        this.count = source["count"];
	        this.values = source["values"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class WorkspaceObjectDetail {
	    id: string;
	    revision?: string;
	    type: string;
	    title: string;
	    module?: string;
	    project?: string;
	    description?: string;
	    parentId?: string;
	    status?: string;
	    tags?: string[];
	    icon?: string;
	    body?: string;
	    typeLabel?: string;
	    typeIcon?: string;
	    tabs: WorkspaceObjectTab[];
	    bare?: boolean;
	    capabilities?: string[];
	    capabilityInfo?: WorkspaceCapabilityInfo[];
	    createdAt?: string;
	    updatedAt?: string;
	    derivedFrom?: string;
	    aspects?: WorkspaceObjectAspect[];
	    badges?: WorkspaceBadge[];
	    policies?: WorkspacePolicy[];
	    actions: WorkspaceQuickAction[];
	    quickActions?: WorkspaceQuickAction[];
	    links?: WorkspaceObjectLink[];
	    children?: WorkspaceObjectChildGroup[];
	    errors?: WorkspaceNodeDiagnostic[];
	    error?: string;
	    errorMsg?: usermsg.Msg;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceObjectDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.revision = source["revision"];
	        this.type = source["type"];
	        this.title = source["title"];
	        this.module = source["module"];
	        this.project = source["project"];
	        this.description = source["description"];
	        this.parentId = source["parentId"];
	        this.status = source["status"];
	        this.tags = source["tags"];
	        this.icon = source["icon"];
	        this.body = source["body"];
	        this.typeLabel = source["typeLabel"];
	        this.typeIcon = source["typeIcon"];
	        this.tabs = this.convertValues(source["tabs"], WorkspaceObjectTab);
	        this.bare = source["bare"];
	        this.capabilities = source["capabilities"];
	        this.capabilityInfo = this.convertValues(source["capabilityInfo"], WorkspaceCapabilityInfo);
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	        this.derivedFrom = source["derivedFrom"];
	        this.aspects = this.convertValues(source["aspects"], WorkspaceObjectAspect);
	        this.badges = this.convertValues(source["badges"], WorkspaceBadge);
	        this.policies = this.convertValues(source["policies"], WorkspacePolicy);
	        this.actions = this.convertValues(source["actions"], WorkspaceQuickAction);
	        this.quickActions = this.convertValues(source["quickActions"], WorkspaceQuickAction);
	        this.links = this.convertValues(source["links"], WorkspaceObjectLink);
	        this.children = this.convertValues(source["children"], WorkspaceObjectChildGroup);
	        this.errors = this.convertValues(source["errors"], WorkspaceNodeDiagnostic);
	        this.error = source["error"];
	        this.errorMsg = this.convertValues(source["errorMsg"], usermsg.Msg);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class WorkspaceUnreadableFile {
	    path: string;
	    problems: string[];
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceUnreadableFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.problems = source["problems"];
	    }
	}
	export class WorkspaceObjectInError {
	    id: string;
	    type?: string;
	    title?: string;
	    module?: string;
	    path: string;
	    errors: WorkspaceNodeDiagnostic[];
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceObjectInError(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.title = source["title"];
	        this.module = source["module"];
	        this.path = source["path"];
	        this.errors = this.convertValues(source["errors"], WorkspaceNodeDiagnostic);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class WorkspaceObjectErrors {
	    objects: WorkspaceObjectInError[];
	    unreadable?: WorkspaceUnreadableFile[];
	    total: number;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceObjectErrors(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.objects = this.convertValues(source["objects"], WorkspaceObjectInError);
	        this.unreadable = this.convertValues(source["unreadable"], WorkspaceUnreadableFile);
	        this.total = source["total"];
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class WorkspaceObjectFindResult {
	    objects: WorkspaceObject[];
	    resolvedId?: string;
	    notMounted?: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceObjectFindResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.objects = this.convertValues(source["objects"], WorkspaceObject);
	        this.resolvedId = source["resolvedId"];
	        this.notMounted = source["notMounted"];
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class WorkspaceObjectInboundResult {
	    from: string[];
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceObjectInboundResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.from = source["from"];
	        this.error = source["error"];
	    }
	}
	export class WorkspaceObjectLifecycles {
	    lifecycles: api.LifecycleOptions[];
	    message?: string;
	    messageMsg?: usermsg.Msg;
	    error?: string;
	    errorMsg?: usermsg.Msg;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceObjectLifecycles(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.lifecycles = this.convertValues(source["lifecycles"], api.LifecycleOptions);
	        this.message = source["message"];
	        this.messageMsg = this.convertValues(source["messageMsg"], usermsg.Msg);
	        this.error = source["error"];
	        this.errorMsg = this.convertValues(source["errorMsg"], usermsg.Msg);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class WorkspaceObjectMutationResult {
	    object: WorkspaceObject;
	    revision?: string;
	    error?: string;
	    errorMsg?: usermsg.Msg;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceObjectMutationResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.object = this.convertValues(source["object"], WorkspaceObject);
	        this.revision = source["revision"];
	        this.error = source["error"];
	        this.errorMsg = this.convertValues(source["errorMsg"], usermsg.Msg);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class WorkspaceObjectTypeDetail {
	    name: string;
	    label?: string;
	    description?: string;
	    icon?: string;
	    capabilities?: string[];
	    fieldsDataTypeUri?: string;
	    allowedChildren?: string[];
	    abstract?: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceObjectTypeDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.label = source["label"];
	        this.description = source["description"];
	        this.icon = source["icon"];
	        this.capabilities = source["capabilities"];
	        this.fieldsDataTypeUri = source["fieldsDataTypeUri"];
	        this.allowedChildren = source["allowedChildren"];
	        this.abstract = source["abstract"];
	        this.error = source["error"];
	    }
	}
	
	
	export class WorkspaceRunActionResult {
	    uri: string;
	    taskId?: string;
	    panel?: string;
	    value?: string;
	    error?: string;
	    errorMsg?: usermsg.Msg;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceRunActionResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.uri = source["uri"];
	        this.taskId = source["taskId"];
	        this.panel = source["panel"];
	        this.value = source["value"];
	        this.error = source["error"];
	        this.errorMsg = this.convertValues(source["errorMsg"], usermsg.Msg);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class WorkspaceRunMethodResult {
	    result?: string;
	    taskId?: string;
	    panel?: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceRunMethodResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.result = source["result"];
	        this.taskId = source["taskId"];
	        this.panel = source["panel"];
	        this.error = source["error"];
	    }
	}

}

export namespace render {
	
	export class Def {
	    kind?: string;
	    mediaType?: string;
	    extension?: string;
	    schema?: any;
	    variant?: string;
	    ui?: Record<string, any>;
	    data?: any;
	    file?: string;
	    mode?: string;
	    dslTree?: any;
	    title?: string;
	
	    static createFrom(source: any = {}) {
	        return new Def(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.mediaType = source["mediaType"];
	        this.extension = source["extension"];
	        this.schema = source["schema"];
	        this.variant = source["variant"];
	        this.ui = source["ui"];
	        this.data = source["data"];
	        this.file = source["file"];
	        this.mode = source["mode"];
	        this.dslTree = source["dslTree"];
	        this.title = source["title"];
	    }
	}
	export class Source {
	    path?: string;
	    uri?: string;
	    layer?: string;
	
	    static createFrom(source: any = {}) {
	        return new Source(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.uri = source["uri"];
	        this.layer = source["layer"];
	    }
	}
	export class Document {
	    mediaType: string;
	    title?: string;
	    content?: any;
	    encoding?: string;
	    source?: Source;
	    meta?: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new Document(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mediaType = source["mediaType"];
	        this.title = source["title"];
	        this.content = source["content"];
	        this.encoding = source["encoding"];
	        this.source = this.convertValues(source["source"], Source);
	        this.meta = source["meta"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ExportFormat {
	    mediaType: string;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new ExportFormat(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mediaType = source["mediaType"];
	        this.label = source["label"];
	    }
	}
	export class EngineCapabilities {
	    stream?: boolean;
	    edit?: boolean;
	    save?: boolean;
	    export?: ExportFormat[];
	
	    static createFrom(source: any = {}) {
	        return new EngineCapabilities(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.stream = source["stream"];
	        this.edit = source["edit"];
	        this.save = source["save"];
	        this.export = this.convertValues(source["export"], ExportFormat);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class EngineFile {
	    suffix: string;
	    mediaType: string;
	
	    static createFrom(source: any = {}) {
	        return new EngineFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.suffix = source["suffix"];
	        this.mediaType = source["mediaType"];
	    }
	}
	

}

export namespace settings {
	
	export class AppearanceSettings {
	    themeMode: string;
	    accentColor: string;
	    fontSize: number;
	    fontFamily: string;
	    monoFamily: string;
	    codeThemeDark: string;
	    codeThemeLight: string;
	
	    static createFrom(source: any = {}) {
	        return new AppearanceSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.themeMode = source["themeMode"];
	        this.accentColor = source["accentColor"];
	        this.fontSize = source["fontSize"];
	        this.fontFamily = source["fontFamily"];
	        this.monoFamily = source["monoFamily"];
	        this.codeThemeDark = source["codeThemeDark"];
	        this.codeThemeLight = source["codeThemeLight"];
	    }
	}
	export class GeneralSettings {
	    level: string;
	    autoScroll: boolean;
	    showToolInputs: boolean;
	    compactMessages: boolean;
	    fullWidthMessages: boolean;
	
	    static createFrom(source: any = {}) {
	        return new GeneralSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.level = source["level"];
	        this.autoScroll = source["autoScroll"];
	        this.showToolInputs = source["showToolInputs"];
	        this.compactMessages = source["compactMessages"];
	        this.fullWidthMessages = source["fullWidthMessages"];
	    }
	}
	export class TreeSettings {
	    hideTypes: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TreeSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hideTypes = source["hideTypes"];
	    }
	}
	export class NotificationSettings {
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new NotificationSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	    }
	}
	export class UniverseSettings {
	    preferredDefault?: string;
	    starred?: Record<string, Array<string>>;
	    sort?: Record<string, string>;
	    removed?: Record<string, Array<string>>;
	
	    static createFrom(source: any = {}) {
	        return new UniverseSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.preferredDefault = source["preferredDefault"];
	        this.starred = source["starred"];
	        this.sort = source["sort"];
	        this.removed = source["removed"];
	    }
	}
	export class GuiSettings {
	    version?: number;
	    appearance: AppearanceSettings;
	    general: GeneralSettings;
	    universes: UniverseSettings;
	    notifications: NotificationSettings;
	    tree: TreeSettings;
	
	    static createFrom(source: any = {}) {
	        return new GuiSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.appearance = this.convertValues(source["appearance"], AppearanceSettings);
	        this.general = this.convertValues(source["general"], GeneralSettings);
	        this.universes = this.convertValues(source["universes"], UniverseSettings);
	        this.notifications = this.convertValues(source["notifications"], NotificationSettings);
	        this.tree = this.convertValues(source["tree"], TreeSettings);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	

}

export namespace supervisor {
	
	export class ActiveUniverseInfo {
	    name: string;
	    homePath: string;
	    startedAtUnix: number;
	    hubKeepsRunning: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ActiveUniverseInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.homePath = source["homePath"];
	        this.startedAtUnix = source["startedAtUnix"];
	        this.hubKeepsRunning = source["hubKeepsRunning"];
	    }
	}

}

export namespace usermsg {
	
	export class Msg {
	    key: string;
	    params?: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new Msg(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.params = source["params"];
	    }
	}

}

