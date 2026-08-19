export namespace main {
	
	export class Project {
	    id: string;
	    path: string;
	    host: string;
	    ssl: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Project(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.path = source["path"];
	        this.host = source["host"];
	        this.ssl = source["ssl"];
	    }
	}

}

