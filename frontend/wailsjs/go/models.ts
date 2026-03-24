export namespace ipc {
	
	export class RGBColorParam {
	    r: number;
	    g: number;
	    b: number;
	
	    static createFrom(source: any = {}) {
	        return new RGBColorParam(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.r = source["r"];
	        this.g = source["g"];
	        this.b = source["b"];
	    }
	}
	export class SetRGBModeParams {
	    mode: string;
	    colors: RGBColorParam[];
	    speed: string;
	    brightness: number;
	
	    static createFrom(source: any = {}) {
	        return new SetRGBModeParams(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.colors = this.convertValues(source["colors"], RGBColorParam);
	        this.speed = source["speed"];
	        this.brightness = source["brightness"];
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

export namespace main {
	
	export class FanCurveProfileConfig {
	    name: string;
	    profilePath?: string;
	    fanCurve: types.FanCurvePoint[];
	
	    static createFrom(source: any = {}) {
	        return new FanCurveProfileConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.profilePath = source["profilePath"];
	        this.fanCurve = this.convertValues(source["fanCurve"], types.FanCurvePoint);
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

export namespace types {
	
	export class HotkeyConflict {
	    accelerator: string;
	    scopes: string[];
	    actions: string[];
	    source?: string;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new HotkeyConflict(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.accelerator = source["accelerator"];
	        this.scopes = source["scopes"];
	        this.actions = source["actions"];
	        this.source = source["source"];
	        this.message = source["message"];
	    }
	}
	export class RGBColorConfig {
	    r: number;
	    g: number;
	    b: number;
	
	    static createFrom(source: any = {}) {
	        return new RGBColorConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.r = source["r"];
	        this.g = source["g"];
	        this.b = source["b"];
	    }
	}
	export class RGBConfig {
	    mode: string;
	    colors: RGBColorConfig[];
	    speed: string;
	    brightness: number;
	
	    static createFrom(source: any = {}) {
	        return new RGBConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.colors = this.convertValues(source["colors"], RGBColorConfig);
	        this.speed = source["speed"];
	        this.brightness = source["brightness"];
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
	export class ProcessFanRule {
	    processName: string;
	    profilePath: string;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ProcessFanRule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.processName = source["processName"];
	        this.profilePath = source["profilePath"];
	        this.enabled = source["enabled"];
	    }
	}
	export class HotkeyBinding {
	    action: string;
	    accelerator: string;
	    scope: string;
	    enabled: boolean;
	    editable: boolean;
	    description: string;
	    category: string;
	
	    static createFrom(source: any = {}) {
	        return new HotkeyBinding(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.action = source["action"];
	        this.accelerator = source["accelerator"];
	        this.scope = source["scope"];
	        this.enabled = source["enabled"];
	        this.editable = source["editable"];
	        this.description = source["description"];
	        this.category = source["category"];
	    }
	}
	export class HotkeyConfig {
	    enabled: boolean;
	    global: HotkeyBinding[];
	    inApp: HotkeyBinding[];
	
	    static createFrom(source: any = {}) {
	        return new HotkeyConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.global = this.convertValues(source["global"], HotkeyBinding);
	        this.inApp = this.convertValues(source["inApp"], HotkeyBinding);
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
	export class FanCurvePoint {
	    temperature: number;
	    rpm: number;
	    offset: number;
	
	    static createFrom(source: any = {}) {
	        return new FanCurvePoint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.temperature = source["temperature"];
	        this.rpm = source["rpm"];
	        this.offset = source["offset"];
	    }
	}
	export class AppConfig {
	    autoControl: boolean;
	    fanCurve: FanCurvePoint[];
	    gearLight: boolean;
	    notificationsEnabled: boolean;
	    hotkeys?: HotkeyConfig;
	    powerOnStart: boolean;
	    windowsAutoStart: boolean;
	    monitorAutoStart: boolean;
	    smartStartStop: string;
	    brightness: number;
	    tempUpdateRate: number;
	    tempSampleCount: number;
	    configPath: string;
	    manualGear: string;
	    manualLevel: string;
	    debugMode: boolean;
	    guiMonitoring: boolean;
	    customSpeedEnabled: boolean;
	    customSpeedRPM: number;
	    ignoreDeviceOnReconnect: boolean;
	    fanCurveOffsetEnabled: boolean;
	    processSwitchEnabled: boolean;
	    processSwitchInterval: number;
	    processSwitchRules: ProcessFanRule[];
	    rgbConfig?: RGBConfig;
	    hotkeyConflicts?: HotkeyConflict[];
	
	    static createFrom(source: any = {}) {
	        return new AppConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.autoControl = source["autoControl"];
	        this.fanCurve = this.convertValues(source["fanCurve"], FanCurvePoint);
	        this.gearLight = source["gearLight"];
	        this.notificationsEnabled = source["notificationsEnabled"];
	        this.hotkeys = this.convertValues(source["hotkeys"], HotkeyConfig);
	        this.powerOnStart = source["powerOnStart"];
	        this.windowsAutoStart = source["windowsAutoStart"];
	        this.monitorAutoStart = source["monitorAutoStart"];
	        this.smartStartStop = source["smartStartStop"];
	        this.brightness = source["brightness"];
	        this.tempUpdateRate = source["tempUpdateRate"];
	        this.tempSampleCount = source["tempSampleCount"];
	        this.configPath = source["configPath"];
	        this.manualGear = source["manualGear"];
	        this.manualLevel = source["manualLevel"];
	        this.debugMode = source["debugMode"];
	        this.guiMonitoring = source["guiMonitoring"];
	        this.customSpeedEnabled = source["customSpeedEnabled"];
	        this.customSpeedRPM = source["customSpeedRPM"];
	        this.ignoreDeviceOnReconnect = source["ignoreDeviceOnReconnect"];
	        this.fanCurveOffsetEnabled = source["fanCurveOffsetEnabled"];
	        this.processSwitchEnabled = source["processSwitchEnabled"];
	        this.processSwitchInterval = source["processSwitchInterval"];
	        this.processSwitchRules = this.convertValues(source["processSwitchRules"], ProcessFanRule);
	        this.rgbConfig = this.convertValues(source["rgbConfig"], RGBConfig);
	        this.hotkeyConflicts = this.convertValues(source["hotkeyConflicts"], HotkeyConflict);
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
	export class BridgeTemperatureData {
	    cpuTemp: number;
	    gpuTemp: number;
	    maxTemp: number;
	    updateTime: number;
	    success: boolean;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new BridgeTemperatureData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cpuTemp = source["cpuTemp"];
	        this.gpuTemp = source["gpuTemp"];
	        this.maxTemp = source["maxTemp"];
	        this.updateTime = source["updateTime"];
	        this.success = source["success"];
	        this.error = source["error"];
	    }
	}
	
	export class FanData {
	    reportId: number;
	    magicSync: number;
	    command: number;
	    status: number;
	    gearSettings: number;
	    currentMode: number;
	    reserved1: number;
	    currentRpm: number;
	    targetRpm: number;
	    maxGear: string;
	    setGear: string;
	    workMode: string;
	
	    static createFrom(source: any = {}) {
	        return new FanData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.reportId = source["reportId"];
	        this.magicSync = source["magicSync"];
	        this.command = source["command"];
	        this.status = source["status"];
	        this.gearSettings = source["gearSettings"];
	        this.currentMode = source["currentMode"];
	        this.reserved1 = source["reserved1"];
	        this.currentRpm = source["currentRpm"];
	        this.targetRpm = source["targetRpm"];
	        this.maxGear = source["maxGear"];
	        this.setGear = source["setGear"];
	        this.workMode = source["workMode"];
	    }
	}
	
	
	
	
	
	
	export class TemperatureData {
	    cpuTemp: number;
	    gpuTemp: number;
	    maxTemp: number;
	    updateTime: number;
	    bridgeOk: boolean;
	    bridgeMessage: string;
	    autoOffset: number;
	    engineState: string;
	
	    static createFrom(source: any = {}) {
	        return new TemperatureData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cpuTemp = source["cpuTemp"];
	        this.gpuTemp = source["gpuTemp"];
	        this.maxTemp = source["maxTemp"];
	        this.updateTime = source["updateTime"];
	        this.bridgeOk = source["bridgeOk"];
	        this.bridgeMessage = source["bridgeMessage"];
	        this.autoOffset = source["autoOffset"];
	        this.engineState = source["engineState"];
	    }
	}

}

