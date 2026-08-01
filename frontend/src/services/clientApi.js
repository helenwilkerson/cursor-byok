import {
  GetState,
  LoadUserConfig,
  SaveUserConfig,
  StartProxy,
  StopProxy,
} from "@bindings/cursor/internal/bridge/proxyservice.js";
import { GetHomeMetricsSummary } from "@bindings/cursor/internal/bridge/metricsservice.js";
import {
  GetAppVersion,
  GetModelEditorContext,
  OpenConfigWindow,
  OpenHistoryWindow,
  OpenModelConfigWindow,
  OpenModelEditorWindow,
  OpenUpstreamReleases,
} from "@bindings/cursor/internal/bridge/windowservice.js";
import { Call } from "@wailsio/runtime";

const API_LOG_PREFIX = "[clientApi]";
const PROXY_SERVICE_NAME = "cursor/internal/bridge.ProxyService";
const STARTUP_SERVICE_NAME = "cursor/internal/bridge.StartupService";

function logSuccess(name, payload, result) {
  console.log(`${API_LOG_PREFIX} ${name} response`, {
    payload,
    result,
  });
}

function logError(name, payload, error) {
  console.error(`${API_LOG_PREFIX} ${name} error`, {
    payload,
    error,
  });
}

function withApiLogging(name, payload, runner) {
  return Promise.resolve()
    .then(() => runner())
    .then((result) => {
      logSuccess(name, payload, result);
      return result;
    })
    .catch((error) => {
      logError(name, payload, error);
      throw error;
    });
}

export function loadUserConfig() {
  return withApiLogging("LoadUserConfig", undefined, () => LoadUserConfig());
}

export function saveUserConfig(payload) {
  return withApiLogging("SaveUserConfig", payload, () => SaveUserConfig(payload));
}

export function getProxyState() {
  return withApiLogging("GetState", undefined, () => GetState());
}

// lyh用cursor修改 2026-08-01：通过桌面桥接读取真实启动项状态，避免前端缓存与 Windows 设置不一致。
/**
 * 读取当前程序的开机启动状态。
 * @returns {Promise<{supported: boolean, enabled: boolean}>} 返回平台支持状态和系统中的实际启用状态。
 */
export function getStartupStatus() {
  return withApiLogging("GetStartupStatus", undefined, () =>
    Call.ByName(`${STARTUP_SERVICE_NAME}.GetStatus`),
  );
}

// lyh用cursor修改 2026-08-01：由后端统一增删 Windows 启动项，避免浏览器界面直接承担平台操作。
/**
 * 设置当前程序是否随系统登录自动启动。
 * @param {boolean} enabled 是否添加当前用户的开机启动项。
 * @returns {Promise<{supported: boolean, enabled: boolean}>} 返回操作后的系统实际状态。
 */
export function setStartupEnabled(enabled) {
  return withApiLogging("SetStartupEnabled", { enabled }, () =>
    Call.ByName(`${STARTUP_SERVICE_NAME}.SetEnabled`, enabled),
  );
}

export function getHomeMetricsSummary() {
  return withApiLogging("GetHomeMetricsSummary", undefined, () => GetHomeMetricsSummary());
}

export function startProxyService() {
  return withApiLogging("StartProxy", undefined, () => StartProxy());
}

export function stopProxyService() {
  return withApiLogging("StopProxy", undefined, () => StopProxy());
}

export function openLogsDirectory() {
  return withApiLogging("OpenHistoryWindow", undefined, () => OpenHistoryWindow());
}

export function openConfigWindow() {
  return withApiLogging("OpenConfigWindow", undefined, () => OpenConfigWindow());
}

export function getAppVersion() {
  return withApiLogging("GetAppVersion", undefined, () => GetAppVersion());
}

export function openUpstreamReleases() {
  return withApiLogging("OpenUpstreamReleases", undefined, () => OpenUpstreamReleases());
}

export function openModelConfig() {
  return withApiLogging("OpenModelConfigWindow", undefined, () => OpenModelConfigWindow());
}

export function openModelEditor(index, adapterJSON) {
  return withApiLogging("OpenModelEditorWindow", { index, adapterJSON }, () =>
    OpenModelEditorWindow(index, adapterJSON),
  );
}

export function getModelEditorContext() {
  return withApiLogging("GetModelEditorContext", undefined, () => GetModelEditorContext());
}

export function testModelAdapter(adapter) {
  return Call.ByName(`${PROXY_SERVICE_NAME}.TestModelAdapter`, adapter).then(
    (result) => {
      logSuccess("TestModelAdapter", adapter, result);
      return result;
    },
    (error) => {
      logError("TestModelAdapter", adapter, error);
      throw error;
    },
  );
}

export function getModelAdapterTestResults() {
  return withApiLogging("GetModelAdapterTestResults", undefined, () =>
    Call.ByName(`${PROXY_SERVICE_NAME}.GetModelAdapterTestResults`),
  );
}
