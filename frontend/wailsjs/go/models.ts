export namespace main {
	
	export class AppStatus {
	    configured: boolean;
	    connected: boolean;
	    serverUrl: string;
	    view: string;
	    error: string;
	    minPriority: number;
	
	    static createFrom(source: any = {}) {
	        return new AppStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.configured = source["configured"];
	        this.connected = source["connected"];
	        this.serverUrl = source["serverUrl"];
	        this.view = source["view"];
	        this.error = source["error"];
	        this.minPriority = source["minPriority"];
	    }
	}

}

