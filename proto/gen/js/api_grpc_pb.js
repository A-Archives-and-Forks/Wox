// GENERATED CODE -- DO NOT EDIT!

'use strict';
var grpc = require('@grpc/grpc-js');
var api_pb = require('./api_pb.js');
var common_pb = require('./common_pb.js');

function serialize_wox_plugin_AIChatRequest(arg) {
  if (!(arg instanceof api_pb.AIChatRequest)) {
    throw new Error('Expected argument of type wox.plugin.AIChatRequest');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_wox_plugin_AIChatRequest(buffer_arg) {
  return api_pb.AIChatRequest.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_wox_plugin_AIChatResponse(arg) {
  if (!(arg instanceof api_pb.AIChatResponse)) {
    throw new Error('Expected argument of type wox.plugin.AIChatResponse');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_wox_plugin_AIChatResponse(buffer_arg) {
  return api_pb.AIChatResponse.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_wox_plugin_ChangeQueryRequest(arg) {
  if (!(arg instanceof api_pb.ChangeQueryRequest)) {
    throw new Error('Expected argument of type wox.plugin.ChangeQueryRequest');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_wox_plugin_ChangeQueryRequest(buffer_arg) {
  return api_pb.ChangeQueryRequest.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_wox_plugin_DeepLinkEvent(arg) {
  if (!(arg instanceof api_pb.DeepLinkEvent)) {
    throw new Error('Expected argument of type wox.plugin.DeepLinkEvent');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_wox_plugin_DeepLinkEvent(buffer_arg) {
  return api_pb.DeepLinkEvent.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_wox_plugin_DynamicSettingEvent(arg) {
  if (!(arg instanceof api_pb.DynamicSettingEvent)) {
    throw new Error('Expected argument of type wox.plugin.DynamicSettingEvent');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_wox_plugin_DynamicSettingEvent(buffer_arg) {
  return api_pb.DynamicSettingEvent.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_wox_plugin_Empty(arg) {
  if (!(arg instanceof common_pb.Empty)) {
    throw new Error('Expected argument of type wox.plugin.Empty');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_wox_plugin_Empty(buffer_arg) {
  return common_pb.Empty.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_wox_plugin_LogRequest(arg) {
  if (!(arg instanceof api_pb.LogRequest)) {
    throw new Error('Expected argument of type wox.plugin.LogRequest');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_wox_plugin_LogRequest(buffer_arg) {
  return api_pb.LogRequest.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_wox_plugin_NotifyRequest(arg) {
  if (!(arg instanceof api_pb.NotifyRequest)) {
    throw new Error('Expected argument of type wox.plugin.NotifyRequest');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_wox_plugin_NotifyRequest(buffer_arg) {
  return api_pb.NotifyRequest.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_wox_plugin_RegisterQueryCommandsRequest(arg) {
  if (!(arg instanceof api_pb.RegisterQueryCommandsRequest)) {
    throw new Error('Expected argument of type wox.plugin.RegisterQueryCommandsRequest');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_wox_plugin_RegisterQueryCommandsRequest(buffer_arg) {
  return api_pb.RegisterQueryCommandsRequest.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_wox_plugin_SaveSettingRequest(arg) {
  if (!(arg instanceof api_pb.SaveSettingRequest)) {
    throw new Error('Expected argument of type wox.plugin.SaveSettingRequest');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_wox_plugin_SaveSettingRequest(buffer_arg) {
  return api_pb.SaveSettingRequest.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_wox_plugin_SettingChangedEvent(arg) {
  if (!(arg instanceof api_pb.SettingChangedEvent)) {
    throw new Error('Expected argument of type wox.plugin.SettingChangedEvent');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_wox_plugin_SettingChangedEvent(buffer_arg) {
  return api_pb.SettingChangedEvent.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_wox_plugin_SettingRequest(arg) {
  if (!(arg instanceof api_pb.SettingRequest)) {
    throw new Error('Expected argument of type wox.plugin.SettingRequest');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_wox_plugin_SettingRequest(buffer_arg) {
  return api_pb.SettingRequest.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_wox_plugin_SettingResponse(arg) {
  if (!(arg instanceof api_pb.SettingResponse)) {
    throw new Error('Expected argument of type wox.plugin.SettingResponse');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_wox_plugin_SettingResponse(buffer_arg) {
  return api_pb.SettingResponse.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_wox_plugin_TranslationRequest(arg) {
  if (!(arg instanceof api_pb.TranslationRequest)) {
    throw new Error('Expected argument of type wox.plugin.TranslationRequest');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_wox_plugin_TranslationRequest(buffer_arg) {
  return api_pb.TranslationRequest.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_wox_plugin_TranslationResponse(arg) {
  if (!(arg instanceof api_pb.TranslationResponse)) {
    throw new Error('Expected argument of type wox.plugin.TranslationResponse');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_wox_plugin_TranslationResponse(buffer_arg) {
  return api_pb.TranslationResponse.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_wox_plugin_UnloadEvent(arg) {
  if (!(arg instanceof api_pb.UnloadEvent)) {
    throw new Error('Expected argument of type wox.plugin.UnloadEvent');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_wox_plugin_UnloadEvent(buffer_arg) {
  return api_pb.UnloadEvent.deserializeBinary(new Uint8Array(buffer_arg));
}


// API interface for plugin to interact with Wox
var PublicAPIService = exports.PublicAPIService = {
  // Change the query in search box
changeQuery: {
    path: '/wox.plugin.PublicAPI/ChangeQuery',
    requestStream: false,
    responseStream: false,
    requestType: api_pb.ChangeQueryRequest,
    responseType: common_pb.Empty,
    requestSerialize: serialize_wox_plugin_ChangeQueryRequest,
    requestDeserialize: deserialize_wox_plugin_ChangeQueryRequest,
    responseSerialize: serialize_wox_plugin_Empty,
    responseDeserialize: deserialize_wox_plugin_Empty,
  },
  // Hide Wox window
hideApp: {
    path: '/wox.plugin.PublicAPI/HideApp',
    requestStream: false,
    responseStream: false,
    requestType: common_pb.Empty,
    responseType: common_pb.Empty,
    requestSerialize: serialize_wox_plugin_Empty,
    requestDeserialize: deserialize_wox_plugin_Empty,
    responseSerialize: serialize_wox_plugin_Empty,
    responseDeserialize: deserialize_wox_plugin_Empty,
  },
  // Show Wox window
showApp: {
    path: '/wox.plugin.PublicAPI/ShowApp',
    requestStream: false,
    responseStream: false,
    requestType: common_pb.Empty,
    responseType: common_pb.Empty,
    requestSerialize: serialize_wox_plugin_Empty,
    requestDeserialize: deserialize_wox_plugin_Empty,
    responseSerialize: serialize_wox_plugin_Empty,
    responseDeserialize: deserialize_wox_plugin_Empty,
  },
  // Show notification
notify: {
    path: '/wox.plugin.PublicAPI/Notify',
    requestStream: false,
    responseStream: false,
    requestType: api_pb.NotifyRequest,
    responseType: common_pb.Empty,
    requestSerialize: serialize_wox_plugin_NotifyRequest,
    requestDeserialize: deserialize_wox_plugin_NotifyRequest,
    responseSerialize: serialize_wox_plugin_Empty,
    responseDeserialize: deserialize_wox_plugin_Empty,
  },
  // Log message
log: {
    path: '/wox.plugin.PublicAPI/Log',
    requestStream: false,
    responseStream: false,
    requestType: api_pb.LogRequest,
    responseType: common_pb.Empty,
    requestSerialize: serialize_wox_plugin_LogRequest,
    requestDeserialize: deserialize_wox_plugin_LogRequest,
    responseSerialize: serialize_wox_plugin_Empty,
    responseDeserialize: deserialize_wox_plugin_Empty,
  },
  // Get translation
getTranslation: {
    path: '/wox.plugin.PublicAPI/GetTranslation',
    requestStream: false,
    responseStream: false,
    requestType: api_pb.TranslationRequest,
    responseType: api_pb.TranslationResponse,
    requestSerialize: serialize_wox_plugin_TranslationRequest,
    requestDeserialize: deserialize_wox_plugin_TranslationRequest,
    responseSerialize: serialize_wox_plugin_TranslationResponse,
    responseDeserialize: deserialize_wox_plugin_TranslationResponse,
  },
  // Get setting value
getSetting: {
    path: '/wox.plugin.PublicAPI/GetSetting',
    requestStream: false,
    responseStream: false,
    requestType: api_pb.SettingRequest,
    responseType: api_pb.SettingResponse,
    requestSerialize: serialize_wox_plugin_SettingRequest,
    requestDeserialize: deserialize_wox_plugin_SettingRequest,
    responseSerialize: serialize_wox_plugin_SettingResponse,
    responseDeserialize: deserialize_wox_plugin_SettingResponse,
  },
  // Save setting value
saveSetting: {
    path: '/wox.plugin.PublicAPI/SaveSetting',
    requestStream: false,
    responseStream: false,
    requestType: api_pb.SaveSettingRequest,
    responseType: common_pb.Empty,
    requestSerialize: serialize_wox_plugin_SaveSettingRequest,
    requestDeserialize: deserialize_wox_plugin_SaveSettingRequest,
    responseSerialize: serialize_wox_plugin_Empty,
    responseDeserialize: deserialize_wox_plugin_Empty,
  },
  // Register query commands
registerQueryCommands: {
    path: '/wox.plugin.PublicAPI/RegisterQueryCommands',
    requestStream: false,
    responseStream: false,
    requestType: api_pb.RegisterQueryCommandsRequest,
    responseType: common_pb.Empty,
    requestSerialize: serialize_wox_plugin_RegisterQueryCommandsRequest,
    requestDeserialize: deserialize_wox_plugin_RegisterQueryCommandsRequest,
    responseSerialize: serialize_wox_plugin_Empty,
    responseDeserialize: deserialize_wox_plugin_Empty,
  },
  // AI chat stream
aIChatStream: {
    path: '/wox.plugin.PublicAPI/AIChatStream',
    requestStream: false,
    responseStream: true,
    requestType: api_pb.AIChatRequest,
    responseType: api_pb.AIChatResponse,
    requestSerialize: serialize_wox_plugin_AIChatRequest,
    requestDeserialize: deserialize_wox_plugin_AIChatRequest,
    responseSerialize: serialize_wox_plugin_AIChatResponse,
    responseDeserialize: deserialize_wox_plugin_AIChatResponse,
  },
  // Register setting changed callback
onSettingChanged: {
    path: '/wox.plugin.PublicAPI/OnSettingChanged',
    requestStream: false,
    responseStream: true,
    requestType: common_pb.Empty,
    responseType: api_pb.SettingChangedEvent,
    requestSerialize: serialize_wox_plugin_Empty,
    requestDeserialize: deserialize_wox_plugin_Empty,
    responseSerialize: serialize_wox_plugin_SettingChangedEvent,
    responseDeserialize: deserialize_wox_plugin_SettingChangedEvent,
  },
  // Register dynamic setting callback
onGetDynamicSetting: {
    path: '/wox.plugin.PublicAPI/OnGetDynamicSetting',
    requestStream: false,
    responseStream: true,
    requestType: common_pb.Empty,
    responseType: api_pb.DynamicSettingEvent,
    requestSerialize: serialize_wox_plugin_Empty,
    requestDeserialize: deserialize_wox_plugin_Empty,
    responseSerialize: serialize_wox_plugin_DynamicSettingEvent,
    responseDeserialize: deserialize_wox_plugin_DynamicSettingEvent,
  },
  // Register deep link callback
onDeepLink: {
    path: '/wox.plugin.PublicAPI/OnDeepLink',
    requestStream: false,
    responseStream: true,
    requestType: common_pb.Empty,
    responseType: api_pb.DeepLinkEvent,
    requestSerialize: serialize_wox_plugin_Empty,
    requestDeserialize: deserialize_wox_plugin_Empty,
    responseSerialize: serialize_wox_plugin_DeepLinkEvent,
    responseDeserialize: deserialize_wox_plugin_DeepLinkEvent,
  },
  // Register unload callback
onUnload: {
    path: '/wox.plugin.PublicAPI/OnUnload',
    requestStream: false,
    responseStream: true,
    requestType: common_pb.Empty,
    responseType: api_pb.UnloadEvent,
    requestSerialize: serialize_wox_plugin_Empty,
    requestDeserialize: deserialize_wox_plugin_Empty,
    responseSerialize: serialize_wox_plugin_UnloadEvent,
    responseDeserialize: deserialize_wox_plugin_UnloadEvent,
  },
};

exports.PublicAPIClient = grpc.makeGenericClientConstructor(PublicAPIService);
