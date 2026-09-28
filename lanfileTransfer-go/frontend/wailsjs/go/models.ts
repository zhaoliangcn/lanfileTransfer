export namespace config {
	
	export class Config {
	    defaultSavePath: string;
	    autoReceive: boolean;
	    maxConcurrentTransfers: number;
	    chunkSize: number;
	    listenPort: number;
	    serviceName: string;
	    serviceType: string;
	    discoveryInterval: number;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.defaultSavePath = source["defaultSavePath"];
	        this.autoReceive = source["autoReceive"];
	        this.maxConcurrentTransfers = source["maxConcurrentTransfers"];
	        this.chunkSize = source["chunkSize"];
	        this.listenPort = source["listenPort"];
	        this.serviceName = source["serviceName"];
	        this.serviceType = source["serviceType"];
	        this.discoveryInterval = source["discoveryInterval"];
	    }
	}

}

export namespace discovery {
	
	export class Peer {
	    id: string;
	    name: string;
	    ip: string;
	    port: number;
	    online: boolean;
	    lastSeen: string;
	    manual?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Peer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.ip = source["ip"];
	        this.port = source["port"];
	        this.online = source["online"];
	        this.lastSeen = source["lastSeen"];
	        this.manual = source["manual"];
	    }
	}

}

export namespace file {
	
	export class FileInfo {
	    name: string;
	    path: string;
	    size: number;
	    isDir: boolean;
	    modTime: string;
	
	    static createFrom(source: any = {}) {
	        return new FileInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.size = source["size"];
	        this.isDir = source["isDir"];
	        this.modTime = source["modTime"];
	    }
	}

}

export namespace transfer {
	
	export class TransferTask {
	    id: string;
	    peerId: string;
	    peerName: string;
	    peerAddr: string;
	    fileName: string;
	    filePath: string;
	    fileSize: number;
	    bytesTransferred: number;
	    type: string;
	    status: string;
	    isSender: boolean;
	    startTime: string;
	    endTime: string;
	    speed: number;
	    progress: number;
	    error?: string;
	    checkpointPath?: string;
	    relativePath?: string;
	
	    static createFrom(source: any = {}) {
	        return new TransferTask(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.peerId = source["peerId"];
	        this.peerName = source["peerName"];
	        this.peerAddr = source["peerAddr"];
	        this.fileName = source["fileName"];
	        this.filePath = source["filePath"];
	        this.fileSize = source["fileSize"];
	        this.bytesTransferred = source["bytesTransferred"];
	        this.type = source["type"];
	        this.status = source["status"];
	        this.isSender = source["isSender"];
	        this.startTime = source["startTime"];
	        this.endTime = source["endTime"];
	        this.speed = source["speed"];
	        this.progress = source["progress"];
	        this.error = source["error"];
	        this.checkpointPath = source["checkpointPath"];
	        this.relativePath = source["relativePath"];
	    }
	}

}

