// package: wox.plugin
// file: common.proto

/* tslint:disable */
/* eslint-disable */

import * as jspb from "google-protobuf";

export class Empty extends jspb.Message { 

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): Empty.AsObject;
    static toObject(includeInstance: boolean, msg: Empty): Empty.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: Empty, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): Empty;
    static deserializeBinaryFromReader(message: Empty, reader: jspb.BinaryReader): Empty;
}

export namespace Empty {
    export type AsObject = {
    }
}

export class WoxImage extends jspb.Message { 
    getImageType(): WoxImage.ImageType;
    setImageType(value: WoxImage.ImageType): WoxImage;
    getImageData(): string;
    setImageData(value: string): WoxImage;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): WoxImage.AsObject;
    static toObject(includeInstance: boolean, msg: WoxImage): WoxImage.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: WoxImage, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): WoxImage;
    static deserializeBinaryFromReader(message: WoxImage, reader: jspb.BinaryReader): WoxImage;
}

export namespace WoxImage {
    export type AsObject = {
        imageType: WoxImage.ImageType,
        imageData: string,
    }

    export enum ImageType {
    ABSOLUTE = 0,
    RELATIVE = 1,
    BASE64 = 2,
    SVG = 3,
    URL = 4,
    EMOJI = 5,
    LOTTIE = 6,
    }

}

export class WoxPreview extends jspb.Message { 
    getPreviewType(): WoxPreview.PreviewType;
    setPreviewType(value: WoxPreview.PreviewType): WoxPreview;
    getPreviewData(): string;
    setPreviewData(value: string): WoxPreview;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): WoxPreview.AsObject;
    static toObject(includeInstance: boolean, msg: WoxPreview): WoxPreview.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: WoxPreview, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): WoxPreview;
    static deserializeBinaryFromReader(message: WoxPreview, reader: jspb.BinaryReader): WoxPreview;
}

export namespace WoxPreview {
    export type AsObject = {
        previewType: WoxPreview.PreviewType,
        previewData: string,
    }

    export enum PreviewType {
    MARKDOWN = 0,
    TEXT = 1,
    IMAGE = 2,
    URL = 3,
    FILE = 4,
    }

}

export class Selection extends jspb.Message { 
    getType(): Selection.SelectionType;
    setType(value: Selection.SelectionType): Selection;
    getText(): string;
    setText(value: string): Selection;
    clearFilePathsList(): void;
    getFilePathsList(): Array<string>;
    setFilePathsList(value: Array<string>): Selection;
    addFilePaths(value: string, index?: number): string;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): Selection.AsObject;
    static toObject(includeInstance: boolean, msg: Selection): Selection.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: Selection, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): Selection;
    static deserializeBinaryFromReader(message: Selection, reader: jspb.BinaryReader): Selection;
}

export namespace Selection {
    export type AsObject = {
        type: Selection.SelectionType,
        text: string,
        filePathsList: Array<string>,
    }

    export enum SelectionType {
    TEXT = 0,
    FILE = 1,
    }

}

export class QueryEnv extends jspb.Message { 
    getActiveWindowTitle(): string;
    setActiveWindowTitle(value: string): QueryEnv;
    getActiveWindowPid(): number;
    setActiveWindowPid(value: number): QueryEnv;
    getActiveBrowserUrl(): string;
    setActiveBrowserUrl(value: string): QueryEnv;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): QueryEnv.AsObject;
    static toObject(includeInstance: boolean, msg: QueryEnv): QueryEnv.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: QueryEnv, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): QueryEnv;
    static deserializeBinaryFromReader(message: QueryEnv, reader: jspb.BinaryReader): QueryEnv;
}

export namespace QueryEnv {
    export type AsObject = {
        activeWindowTitle: string,
        activeWindowPid: number,
        activeBrowserUrl: string,
    }
}

export class ResultTail extends jspb.Message { 
    getType(): ResultTail.TailType;
    setType(value: ResultTail.TailType): ResultTail;
    getText(): string;
    setText(value: string): ResultTail;

    hasImage(): boolean;
    clearImage(): void;
    getImage(): WoxImage | undefined;
    setImage(value?: WoxImage): ResultTail;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): ResultTail.AsObject;
    static toObject(includeInstance: boolean, msg: ResultTail): ResultTail.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: ResultTail, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): ResultTail;
    static deserializeBinaryFromReader(message: ResultTail, reader: jspb.BinaryReader): ResultTail;
}

export namespace ResultTail {
    export type AsObject = {
        type: ResultTail.TailType,
        text: string,
        image?: WoxImage.AsObject,
    }

    export enum TailType {
    TEXT = 0,
    IMAGE = 1,
    }

}

export class ResultAction extends jspb.Message { 
    getId(): string;
    setId(value: string): ResultAction;
    getName(): string;
    setName(value: string): ResultAction;

    hasIcon(): boolean;
    clearIcon(): void;
    getIcon(): WoxImage | undefined;
    setIcon(value?: WoxImage): ResultAction;
    getIsDefault(): boolean;
    setIsDefault(value: boolean): ResultAction;
    getPreventHideAfterAction(): boolean;
    setPreventHideAfterAction(value: boolean): ResultAction;
    getHotkey(): string;
    setHotkey(value: string): ResultAction;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): ResultAction.AsObject;
    static toObject(includeInstance: boolean, msg: ResultAction): ResultAction.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: ResultAction, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): ResultAction;
    static deserializeBinaryFromReader(message: ResultAction, reader: jspb.BinaryReader): ResultAction;
}

export namespace ResultAction {
    export type AsObject = {
        id: string,
        name: string,
        icon?: WoxImage.AsObject,
        isDefault: boolean,
        preventHideAfterAction: boolean,
        hotkey: string,
    }
}

export class MetadataCommand extends jspb.Message { 
    getCommand(): string;
    setCommand(value: string): MetadataCommand;
    getDescription(): string;
    setDescription(value: string): MetadataCommand;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): MetadataCommand.AsObject;
    static toObject(includeInstance: boolean, msg: MetadataCommand): MetadataCommand.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: MetadataCommand, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): MetadataCommand;
    static deserializeBinaryFromReader(message: MetadataCommand, reader: jspb.BinaryReader): MetadataCommand;
}

export namespace MetadataCommand {
    export type AsObject = {
        command: string,
        description: string,
    }
}

export class MetadataSetting extends jspb.Message { 
    getName(): string;
    setName(value: string): MetadataSetting;
    getType(): string;
    setType(value: string): MetadataSetting;
    getDefaultValue(): string;
    setDefaultValue(value: string): MetadataSetting;
    getDescription(): string;
    setDescription(value: string): MetadataSetting;
    getIsRequired(): boolean;
    setIsRequired(value: boolean): MetadataSetting;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): MetadataSetting.AsObject;
    static toObject(includeInstance: boolean, msg: MetadataSetting): MetadataSetting.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: MetadataSetting, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): MetadataSetting;
    static deserializeBinaryFromReader(message: MetadataSetting, reader: jspb.BinaryReader): MetadataSetting;
}

export namespace MetadataSetting {
    export type AsObject = {
        name: string,
        type: string,
        defaultValue: string,
        description: string,
        isRequired: boolean,
    }
}

export class MapString extends jspb.Message { 

    getDataMap(): jspb.Map<string, string>;
    clearDataMap(): void;

    serializeBinary(): Uint8Array;
    toObject(includeInstance?: boolean): MapString.AsObject;
    static toObject(includeInstance: boolean, msg: MapString): MapString.AsObject;
    static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
    static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
    static serializeBinaryToWriter(message: MapString, writer: jspb.BinaryWriter): void;
    static deserializeBinary(bytes: Uint8Array): MapString;
    static deserializeBinaryFromReader(message: MapString, reader: jspb.BinaryReader): MapString;
}

export namespace MapString {
    export type AsObject = {

        dataMap: Array<[string, string]>,
    }
}
