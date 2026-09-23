export namespace config {
	
	export class HotkeyBinding {
	    id: string;
	    action: string;
	    key_combo: string;
	    key_code: number;
	    modifiers: number;
	
	    static createFrom(source: any = {}) {
	        return new HotkeyBinding(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.action = source["action"];
	        this.key_combo = source["key_combo"];
	        this.key_code = source["key_code"];
	        this.modifiers = source["modifiers"];
	    }
	}
	export class ProcessProfile {
	    id: string;
	    process_name: string;
	    display_name: string;
	    enabled: boolean;
	    hotkeys: HotkeyBinding[];
	    target_app_user_model_id?: string;
	    volume_step_percent?: number;
	
	    static createFrom(source: any = {}) {
	        return new ProcessProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.process_name = source["process_name"];
	        this.display_name = source["display_name"];
	        this.enabled = source["enabled"];
	        this.hotkeys = this.convertValues(source["hotkeys"], HotkeyBinding);
	        this.target_app_user_model_id = source["target_app_user_model_id"];
	        this.volume_step_percent = source["volume_step_percent"];
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
	export class AppConfig {
	    version: string;
	    profiles: ProcessProfile[];
	    global_hotkeys_disabled?: boolean;
	    language?: string;
	    autostart?: boolean;
	    start_minimized?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AppConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.profiles = this.convertValues(source["profiles"], ProcessProfile);
	        this.global_hotkeys_disabled = source["global_hotkeys_disabled"];
	        this.language = source["language"];
	        this.autostart = source["autostart"];
	        this.start_minimized = source["start_minimized"];
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

export namespace hotkey {
	
	export class ConflictBinding {
	    profile_id: string;
	    process_name: string;
	    display_name: string;
	    action: string;
	    hotkey_id: string;
	
	    static createFrom(source: any = {}) {
	        return new ConflictBinding(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profile_id = source["profile_id"];
	        this.process_name = source["process_name"];
	        this.display_name = source["display_name"];
	        this.action = source["action"];
	        this.hotkey_id = source["hotkey_id"];
	    }
	}
	export class Conflict {
	    key_code: number;
	    modifiers: number;
	    bindings: ConflictBinding[];
	
	    static createFrom(source: any = {}) {
	        return new Conflict(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key_code = source["key_code"];
	        this.modifiers = source["modifiers"];
	        this.bindings = this.convertValues(source["bindings"], ConflictBinding);
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
	
	export class RegistrationStatus {
	    profile_id: string;
	    profile_name: string;
	    action: string;
	    key_combo: string;
	    registered: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new RegistrationStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profile_id = source["profile_id"];
	        this.profile_name = source["profile_name"];
	        this.action = source["action"];
	        this.key_combo = source["key_combo"];
	        this.registered = source["registered"];
	        this.error = source["error"];
	    }
	}

}

export namespace main {
	
	export class ProfileState {
	    profile_id: string;
	    has_session: boolean;
	    volume: number;
	    muted: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProfileState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profile_id = source["profile_id"];
	        this.has_session = source["has_session"];
	        this.volume = source["volume"];
	        this.muted = source["muted"];
	        this.error = source["error"];
	    }
	}

}

export namespace process {
	
	export class ProcessInfo {
	    pid: number;
	    process_name: string;
	    display_name: string;
	    icon_base64: string;
	    has_window: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ProcessInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pid = source["pid"];
	        this.process_name = source["process_name"];
	        this.display_name = source["display_name"];
	        this.icon_base64 = source["icon_base64"];
	        this.has_window = source["has_window"];
	    }
	}

}

