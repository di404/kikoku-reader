export namespace main {
	
	export class FontChoice {
	    label: string;
	    family: string;
	
	    static createFrom(source: any = {}) {
	        return new FontChoice(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.family = source["family"];
	    }
	}
	export class Settings {
	    gameDir: string;
	    fontFamily: string;
	    fontFile: string;
	    fontScale: number;
	    lineHeight: number;
	    paraGap: number;
	    boxOpacity: number;
	    music: number;
	    voice: number;
	    effect: number;
	    last: number;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.gameDir = source["gameDir"];
	        this.fontFamily = source["fontFamily"];
	        this.fontFile = source["fontFile"];
	        this.fontScale = source["fontScale"];
	        this.lineHeight = source["lineHeight"];
	        this.paraGap = source["paraGap"];
	        this.boxOpacity = source["boxOpacity"];
	        this.music = source["music"];
	        this.voice = source["voice"];
	        this.effect = source["effect"];
	        this.last = source["last"];
	    }
	}
	export class InitResult {
	    error?: string;
	    scenes: script.Scene[];
	    total: number;
	    settings: Settings;
	    fonts: FontChoice[];
	
	    static createFrom(source: any = {}) {
	        return new InitResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.error = source["error"];
	        this.scenes = this.convertValues(source["scenes"], script.Scene);
	        this.total = source["total"];
	        this.settings = this.convertValues(source["settings"], Settings);
	        this.fonts = this.convertValues(source["fonts"], FontChoice);
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
	
	export class View {
	    index: number;
	    step: script.Step;
	    before: string;
	    scene: number;
	    next: string[];
	
	    static createFrom(source: any = {}) {
	        return new View(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.step = this.convertValues(source["step"], script.Step);
	        this.before = source["before"];
	        this.scene = source["scene"];
	        this.next = source["next"];
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

export namespace script {
	
	export class Layer {
	    kind: string;
	    src?: string;
	    color?: string;
	    z: number;
	    x: number;
	    y: number;
	    w: number;
	    h: number;
	    scale?: number;
	    scaleY?: number;
	    opacity: number;
	    blend?: string;
	    filter?: string;
	    loop?: boolean;
	    children?: Layer[];
	
	    static createFrom(source: any = {}) {
	        return new Layer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.src = source["src"];
	        this.color = source["color"];
	        this.z = source["z"];
	        this.x = source["x"];
	        this.y = source["y"];
	        this.w = source["w"];
	        this.h = source["h"];
	        this.scale = source["scale"];
	        this.scaleY = source["scaleY"];
	        this.opacity = source["opacity"];
	        this.blend = source["blend"];
	        this.filter = source["filter"];
	        this.loop = source["loop"];
	        this.children = this.convertValues(source["children"], Layer);
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
	export class Scene {
	    file: string;
	    first: number;
	    title: string;
	
	    static createFrom(source: any = {}) {
	        return new Scene(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.file = source["file"];
	        this.first = source["first"];
	        this.title = source["title"];
	    }
	}
	export class Sound {
	    name: string;
	    src: string;
	    vol: number;
	    loop: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Sound(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.src = source["src"];
	        this.vol = source["vol"];
	        this.loop = source["loop"];
	    }
	}
	export class Step {
	    scene: string;
	    label?: string;
	    html: string;
	    append?: boolean;
	    speaker?: string;
	    side?: string;
	    voice?: string;
	    layers: Layer[];
	    loops: Sound[];
	    se?: Sound[];
	    movies?: Layer[];
	
	    static createFrom(source: any = {}) {
	        return new Step(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scene = source["scene"];
	        this.label = source["label"];
	        this.html = source["html"];
	        this.append = source["append"];
	        this.speaker = source["speaker"];
	        this.side = source["side"];
	        this.voice = source["voice"];
	        this.layers = this.convertValues(source["layers"], Layer);
	        this.loops = this.convertValues(source["loops"], Sound);
	        this.se = this.convertValues(source["se"], Sound);
	        this.movies = this.convertValues(source["movies"], Layer);
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

