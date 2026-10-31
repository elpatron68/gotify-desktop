export namespace history {
	
	export class Entry {
	    id: string;
	    title: string;
	    body: string;
	    priority: number;
	    app_id: number;
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
	        this.received_at = source["received_at"];
	        this.notified = source["notified"];
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

