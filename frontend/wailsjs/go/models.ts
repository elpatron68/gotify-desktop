export namespace history {
	
	export class Entry {
	    id: string;
	    title: string;
	    body: string;
	    priority: number;
	    app_id: number;
	    // Go type: time
	    received_at: any;
	    notified: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.body = source["body"];
	        this.priority = source["priority"];
	        this.app_id = source["app_id"];
	        this.received_at = this.convertValues(source["received_at"], null);
	        this.notified = source["notified"];
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
	
	export class AppStatus {
	    configured: boolean;
	    connected: boolean;
	    serverUrl: string;
	    username: string;
	    view: string;
	    error: string;
	    minPriority: number;
	    notifyReady: boolean;
	    notifyError: string;
	    lastMessageId: number;
	
	    static createFrom(source: any = {}) {
	        return new AppStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.configured = source["configured"];
	        this.connected = source["connected"];
	        this.serverUrl = source["serverUrl"];
	        this.username = source["username"];
	        this.view = source["view"];
	        this.error = source["error"];
	        this.minPriority = source["minPriority"];
	        this.notifyReady = source["notifyReady"];
	        this.notifyError = source["notifyError"];
	        this.lastMessageId = source["lastMessageId"];
	    }
	}

}

export namespace push {
	
	export class Application {
	    id: number;
	    name: string;
	    description: string;
	    defaultPriority: number;
	
	    static createFrom(source: any = {}) {
	        return new Application(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.defaultPriority = source["defaultPriority"];
	    }
	}

}

