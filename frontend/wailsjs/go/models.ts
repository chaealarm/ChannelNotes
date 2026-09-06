export namespace main {
	
	export class Note {
	    id: string;
	    title: string;
	    name: string;
	    titleLinked: boolean;
	    content: string;
	    contentLoaded?: boolean;
	    updatedAt: string;
	    order: number;
	
	    static createFrom(source: any = {}) {
	        return new Note(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.name = source["name"];
	        this.titleLinked = source["titleLinked"];
	        this.content = source["content"];
	        this.contentLoaded = source["contentLoaded"];
	        this.updatedAt = source["updatedAt"];
	        this.order = source["order"];
	    }
	}
	export class Category {
	    id: string;
	    name: string;
	    order: number;
	    notes: Note[];
	
	    static createFrom(source: any = {}) {
	        return new Category(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.order = source["order"];
	        this.notes = this.convertValues(source["notes"], Note);
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
	export class Channel {
	    id: string;
	    name: string;
	    image: string;
	    groupId: string;
	    order: number;
	    notes?: Note[];
	    categories: Category[];
	
	    static createFrom(source: any = {}) {
	        return new Channel(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.image = source["image"];
	        this.groupId = source["groupId"];
	        this.order = source["order"];
	        this.notes = this.convertValues(source["notes"], Note);
	        this.categories = this.convertValues(source["categories"], Category);
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
	export class Group {
	    id: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new Group(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	    }
	}
	export class GroupBundle {
	    group: Group;
	    channels: Channel[];
	
	    static createFrom(source: any = {}) {
	        return new GroupBundle(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.group = this.convertValues(source["group"], Group);
	        this.channels = this.convertValues(source["channels"], Channel);
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
	export class ImageData {
	    name: string;
	    dataUrl: string;
	
	    static createFrom(source: any = {}) {
	        return new ImageData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.dataUrl = source["dataUrl"];
	    }
	}
	
	export class ReplaceResult {
	    replacements: number;
	    files: number;
	    skippedGroups: string[];
	
	    static createFrom(source: any = {}) {
	        return new ReplaceResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.replacements = source["replacements"];
	        this.files = source["files"];
	        this.skippedGroups = source["skippedGroups"];
	    }
	}
	export class SearchResult {
	    groupId: string;
	    groupName: string;
	    channelId: string;
	    channelName: string;
	    categoryId: string;
	    categoryName: string;
	    noteId: string;
	    noteName: string;
	    snippet: string;
	    matches: number;
	
	    static createFrom(source: any = {}) {
	        return new SearchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.groupId = source["groupId"];
	        this.groupName = source["groupName"];
	        this.channelId = source["channelId"];
	        this.channelName = source["channelName"];
	        this.categoryId = source["categoryId"];
	        this.categoryName = source["categoryName"];
	        this.noteId = source["noteId"];
	        this.noteName = source["noteName"];
	        this.snippet = source["snippet"];
	        this.matches = source["matches"];
	    }
	}
	export class Store {
	    groups: Group[];
	    channels: Channel[];
	    lastGroupId: string;
	    lastChannelId: string;
	    lastCategoryId: string;
	    lastNoteId: string;
	    theme: string;
	    showGroupPopup: boolean;
	    periodicAutoSave: boolean;
	    hideChannels: boolean;
	    hideNotes: boolean;
	    settingsVersion: number;
	
	    static createFrom(source: any = {}) {
	        return new Store(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.groups = this.convertValues(source["groups"], Group);
	        this.channels = this.convertValues(source["channels"], Channel);
	        this.lastGroupId = source["lastGroupId"];
	        this.lastChannelId = source["lastChannelId"];
	        this.lastCategoryId = source["lastCategoryId"];
	        this.lastNoteId = source["lastNoteId"];
	        this.theme = source["theme"];
	        this.showGroupPopup = source["showGroupPopup"];
	        this.periodicAutoSave = source["periodicAutoSave"];
	        this.hideChannels = source["hideChannels"];
	        this.hideNotes = source["hideNotes"];
	        this.settingsVersion = source["settingsVersion"];
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

