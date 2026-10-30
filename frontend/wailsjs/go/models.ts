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

