export namespace main {
	
	export class OSCTargetDTO {
	    host: string;
	    port: number;
	
	    static createFrom(source: any = {}) {
	        return new OSCTargetDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.port = source["port"];
	    }
	}
	export class Problem {
	    code: string;
	    message: string;
	    field?: string;
	    currentRevision?: number;
	
	    static createFrom(source: any = {}) {
	        return new Problem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.code = source["code"];
	        this.message = source["message"];
	        this.field = source["field"];
	        this.currentRevision = source["currentRevision"];
	    }
	}
	export class PluginConfigResponse {
	    revision: number;
	    // Go type: time
	    updatedAt: any;
	    pluginId: string;
	    configRevision: number;
	    data: string;
	    problem?: Problem;
	
	    static createFrom(source: any = {}) {
	        return new PluginConfigResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.revision = source["revision"];
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.pluginId = source["pluginId"];
	        this.configRevision = source["configRevision"];
	        this.data = source["data"];
	        this.problem = this.convertValues(source["problem"], Problem);
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
	export class PluginControlFailureDTO {
	    pluginId: string;
	    operation: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new PluginControlFailureDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pluginId = source["pluginId"];
	        this.operation = source["operation"];
	        this.message = source["message"];
	    }
	}
	export class PluginDTO {
	    id: string;
	    name: string;
	    description: string;
	    version: string;
	    capabilities: string[];
	    enabled: boolean;
	    active: boolean;
	    state: string;
	    configRevision: number;
	    frameRate: number;
	    consecutiveFailures: number;
	    restartCount: number;
	    // Go type: time
	    startedAt?: any;
	    // Go type: time
	    lastHeartbeatAt?: any;
	    // Go type: time
	    lastFrameAt?: any;
	    // Go type: time
	    nextRestartAt?: any;
	    lastError?: string;
	
	    static createFrom(source: any = {}) {
	        return new PluginDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.version = source["version"];
	        this.capabilities = source["capabilities"];
	        this.enabled = source["enabled"];
	        this.active = source["active"];
	        this.state = source["state"];
	        this.configRevision = source["configRevision"];
	        this.frameRate = source["frameRate"];
	        this.consecutiveFailures = source["consecutiveFailures"];
	        this.restartCount = source["restartCount"];
	        this.startedAt = this.convertValues(source["startedAt"], null);
	        this.lastHeartbeatAt = this.convertValues(source["lastHeartbeatAt"], null);
	        this.lastFrameAt = this.convertValues(source["lastFrameAt"], null);
	        this.nextRestartAt = this.convertValues(source["nextRestartAt"], null);
	        this.lastError = source["lastError"];
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
	export class PluginListResponse {
	    revision: number;
	    // Go type: time
	    updatedAt: any;
	    plugins: PluginDTO[];
	    problem?: Problem;
	
	    static createFrom(source: any = {}) {
	        return new PluginListResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.revision = source["revision"];
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.plugins = this.convertValues(source["plugins"], PluginDTO);
	        this.problem = this.convertValues(source["problem"], Problem);
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
	export class PluginMutationResponse {
	    revision: number;
	    // Go type: time
	    updatedAt: any;
	    pluginId: string;
	    problem?: Problem;
	
	    static createFrom(source: any = {}) {
	        return new PluginMutationResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.revision = source["revision"];
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.pluginId = source["pluginId"];
	        this.problem = this.convertValues(source["problem"], Problem);
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
	
	export class RuntimeOSCDTO {
	    running: boolean;
	    connected: boolean;
	    hasTarget: boolean;
	    targetMode: string;
	    target: OSCTargetDTO;
	    lastError?: string;
	
	    static createFrom(source: any = {}) {
	        return new RuntimeOSCDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.connected = source["connected"];
	        this.hasTarget = source["hasTarget"];
	        this.targetMode = source["targetMode"];
	        this.target = this.convertValues(source["target"], OSCTargetDTO);
	        this.lastError = source["lastError"];
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
	export class RuntimeApplicationDTO {
	    lifecycle: string;
	    avatarId: string;
	    planGeneration: number;
	    planStatus: string;
	    planSource: string;
	    configPath: string;
	    configId: string;
	    generationExhausted: boolean;
	    osc: RuntimeOSCDTO;
	    pluginFailures: PluginControlFailureDTO[];
	    planError?: string;
	    runtimeError?: string;
	
	    static createFrom(source: any = {}) {
	        return new RuntimeApplicationDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.lifecycle = source["lifecycle"];
	        this.avatarId = source["avatarId"];
	        this.planGeneration = source["planGeneration"];
	        this.planStatus = source["planStatus"];
	        this.planSource = source["planSource"];
	        this.configPath = source["configPath"];
	        this.configId = source["configId"];
	        this.generationExhausted = source["generationExhausted"];
	        this.osc = this.convertValues(source["osc"], RuntimeOSCDTO);
	        this.pluginFailures = this.convertValues(source["pluginFailures"], PluginControlFailureDTO);
	        this.planError = source["planError"];
	        this.runtimeError = source["runtimeError"];
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
	
	export class RuntimeResponse {
	    revision: number;
	    // Go type: time
	    updatedAt: any;
	    phase: string;
	    platformSupported: boolean;
	    application?: RuntimeApplicationDTO;
	    problem?: Problem;
	
	    static createFrom(source: any = {}) {
	        return new RuntimeResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.revision = source["revision"];
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.phase = source["phase"];
	        this.platformSupported = source["platformSupported"];
	        this.application = this.convertValues(source["application"], RuntimeApplicationDTO);
	        this.problem = this.convertValues(source["problem"], Problem);
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
	export class SettingsResponse {
	    revision: number;
	    // Go type: time
	    updatedAt: any;
	    fileRevision: number;
	    settings: userconfig.Candidate;
	    problem?: Problem;
	
	    static createFrom(source: any = {}) {
	        return new SettingsResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.revision = source["revision"];
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.fileRevision = source["fileRevision"];
	        this.settings = this.convertValues(source["settings"], userconfig.Candidate);
	        this.problem = this.convertValues(source["problem"], Problem);
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
	export class SettingsSaveResponse {
	    revision: number;
	    // Go type: time
	    updatedAt: any;
	    fileRevision: number;
	    settings: userconfig.Candidate;
	    restartRequired: boolean;
	    problem?: Problem;
	
	    static createFrom(source: any = {}) {
	        return new SettingsSaveResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.revision = source["revision"];
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.fileRevision = source["fileRevision"];
	        this.settings = this.convertValues(source["settings"], userconfig.Candidate);
	        this.restartRequired = source["restartRequired"];
	        this.problem = this.convertValues(source["problem"], Problem);
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
	export class SettingsValidationResponse {
	    revision: number;
	    // Go type: time
	    updatedAt: any;
	    settings: userconfig.Candidate;
	    problem?: Problem;
	
	    static createFrom(source: any = {}) {
	        return new SettingsValidationResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.revision = source["revision"];
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.settings = this.convertValues(source["settings"], userconfig.Candidate);
	        this.problem = this.convertValues(source["problem"], Problem);
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

export namespace userconfig {
	
	export class Avatar {
	    oscRoot: string;
	    fallbackPath: string;
	
	    static createFrom(source: any = {}) {
	        return new Avatar(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.oscRoot = source["oscRoot"];
	        this.fallbackPath = source["fallbackPath"];
	    }
	}
	export class Calibration {
	    enabled: boolean;
	    neutral: number;
	    min: number;
	    max: number;
	    gain: number;
	    invert: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Calibration(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.neutral = source["neutral"];
	        this.min = source["min"];
	        this.max = source["max"];
	        this.gain = source["gain"];
	        this.invert = source["invert"];
	    }
	}
	export class OSC {
	    targetMode: string;
	    preferredService: string;
	    manualHost: string;
	    manualPort: number;
	
	    static createFrom(source: any = {}) {
	        return new OSC(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.targetMode = source["targetMode"];
	        this.preferredService = source["preferredService"];
	        this.manualHost = source["manualHost"];
	        this.manualPort = source["manualPort"];
	    }
	}
	export class ProcessingOverride {
	    name: string;
	    channel: ProcessingChannel;
	
	    static createFrom(source: any = {}) {
	        return new ProcessingOverride(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.channel = this.convertValues(source["channel"], ProcessingChannel);
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
	export class Dropout {
	    holdDurationMs: number;
	    decayDurationMs: number;
	    staleAfterMs: number;
	
	    static createFrom(source: any = {}) {
	        return new Dropout(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.holdDurationMs = source["holdDurationMs"];
	        this.decayDurationMs = source["decayDurationMs"];
	        this.staleAfterMs = source["staleAfterMs"];
	    }
	}
	export class Filter {
	    mode: string;
	    emaAlpha: number;
	    minCutoff: number;
	    beta: number;
	    derivativeCutoff: number;
	
	    static createFrom(source: any = {}) {
	        return new Filter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.emaAlpha = source["emaAlpha"];
	        this.minCutoff = source["minCutoff"];
	        this.beta = source["beta"];
	        this.derivativeCutoff = source["derivativeCutoff"];
	    }
	}
	export class Tuning {
	    deadzone: number;
	    gain: number;
	    exponent: number;
	    clampEnabled: boolean;
	    clampMin: number;
	    clampMax: number;
	
	    static createFrom(source: any = {}) {
	        return new Tuning(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.deadzone = source["deadzone"];
	        this.gain = source["gain"];
	        this.exponent = source["exponent"];
	        this.clampEnabled = source["clampEnabled"];
	        this.clampMin = source["clampMin"];
	        this.clampMax = source["clampMax"];
	    }
	}
	export class ProcessingChannel {
	    calibration: Calibration;
	    tuning: Tuning;
	    filter: Filter;
	    dropout: Dropout;
	
	    static createFrom(source: any = {}) {
	        return new ProcessingChannel(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.calibration = this.convertValues(source["calibration"], Calibration);
	        this.tuning = this.convertValues(source["tuning"], Tuning);
	        this.filter = this.convertValues(source["filter"], Filter);
	        this.dropout = this.convertValues(source["dropout"], Dropout);
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
	export class Processing {
	    defaultChannel: ProcessingChannel;
	    overrides: ProcessingOverride[];
	    activeStaleAfterMs: number;
	    mutualExclusion: string[][];
	
	    static createFrom(source: any = {}) {
	        return new Processing(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.defaultChannel = this.convertValues(source["defaultChannel"], ProcessingChannel);
	        this.overrides = this.convertValues(source["overrides"], ProcessingOverride);
	        this.activeStaleAfterMs = source["activeStaleAfterMs"];
	        this.mutualExclusion = source["mutualExclusion"];
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
	export class Plugins {
	    devRoots: string[];
	
	    static createFrom(source: any = {}) {
	        return new Plugins(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.devRoots = source["devRoots"];
	    }
	}
	export class Candidate {
	    avatar: Avatar;
	    plugins: Plugins;
	    processing: Processing;
	    osc: OSC;
	
	    static createFrom(source: any = {}) {
	        return new Candidate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.avatar = this.convertValues(source["avatar"], Avatar);
	        this.plugins = this.convertValues(source["plugins"], Plugins);
	        this.processing = this.convertValues(source["processing"], Processing);
	        this.osc = this.convertValues(source["osc"], OSC);
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

