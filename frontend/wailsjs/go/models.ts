export namespace main {
	
	export class Project {
	    id: string;
	    path: string;
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new Project(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.path = source["path"];
	        this.url = source["url"];
	    }
	}

}

