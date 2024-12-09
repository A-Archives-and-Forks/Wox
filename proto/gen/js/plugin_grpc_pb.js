// GENERATED CODE -- DO NOT EDIT!

'use strict';
var grpc = require('@grpc/grpc-js');
var plugin_pb = require('./plugin_pb.js');
var common_pb = require('./common_pb.js');
var api_pb = require('./api_pb.js');

function serialize_wox_plugin_Empty(arg) {
  if (!(arg instanceof common_pb.Empty)) {
    throw new Error('Expected argument of type wox.plugin.Empty');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_wox_plugin_Empty(buffer_arg) {
  return common_pb.Empty.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_wox_plugin_Metadata(arg) {
  if (!(arg instanceof plugin_pb.Metadata)) {
    throw new Error('Expected argument of type wox.plugin.Metadata');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_wox_plugin_Metadata(buffer_arg) {
  return plugin_pb.Metadata.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_wox_plugin_PluginInitContext(arg) {
  if (!(arg instanceof plugin_pb.PluginInitContext)) {
    throw new Error('Expected argument of type wox.plugin.PluginInitContext');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_wox_plugin_PluginInitContext(buffer_arg) {
  return plugin_pb.PluginInitContext.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_wox_plugin_PluginQueryContext(arg) {
  if (!(arg instanceof plugin_pb.PluginQueryContext)) {
    throw new Error('Expected argument of type wox.plugin.PluginQueryContext');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_wox_plugin_PluginQueryContext(buffer_arg) {
  return plugin_pb.PluginQueryContext.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_wox_plugin_QueryResponse(arg) {
  if (!(arg instanceof plugin_pb.QueryResponse)) {
    throw new Error('Expected argument of type wox.plugin.QueryResponse');
  }
  return Buffer.from(arg.serializeBinary());
}

function deserialize_wox_plugin_QueryResponse(buffer_arg) {
  return plugin_pb.QueryResponse.deserializeBinary(new Uint8Array(buffer_arg));
}


// Core plugin interface definition
var PluginService = exports.PluginService = {
  // Initialize the plugin
init: {
    path: '/wox.plugin.Plugin/Init',
    requestStream: false,
    responseStream: false,
    requestType: plugin_pb.PluginInitContext,
    responseType: common_pb.Empty,
    requestSerialize: serialize_wox_plugin_PluginInitContext,
    requestDeserialize: deserialize_wox_plugin_PluginInitContext,
    responseSerialize: serialize_wox_plugin_Empty,
    responseDeserialize: deserialize_wox_plugin_Empty,
  },
  // Handle search query
query: {
    path: '/wox.plugin.Plugin/Query',
    requestStream: false,
    responseStream: false,
    requestType: plugin_pb.PluginQueryContext,
    responseType: plugin_pb.QueryResponse,
    requestSerialize: serialize_wox_plugin_PluginQueryContext,
    requestDeserialize: deserialize_wox_plugin_PluginQueryContext,
    responseSerialize: serialize_wox_plugin_QueryResponse,
    responseDeserialize: deserialize_wox_plugin_QueryResponse,
  },
};

exports.PluginClient = grpc.makeGenericClientConstructor(PluginService);
// System plugin interface definition
var SystemPluginService = exports.SystemPluginService = {
  // Get plugin metadata
getMetadata: {
    path: '/wox.plugin.SystemPlugin/GetMetadata',
    requestStream: false,
    responseStream: false,
    requestType: common_pb.Empty,
    responseType: plugin_pb.Metadata,
    requestSerialize: serialize_wox_plugin_Empty,
    requestDeserialize: deserialize_wox_plugin_Empty,
    responseSerialize: serialize_wox_plugin_Metadata,
    responseDeserialize: deserialize_wox_plugin_Metadata,
  },
};

exports.SystemPluginClient = grpc.makeGenericClientConstructor(SystemPluginService);
