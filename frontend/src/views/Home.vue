<script setup>
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import Input from "@/components/ui/Input.vue";
import Select from "@/components/ui/Select.vue";
import Switch from "@/components/ui/Switch.vue";
import HomeMetricsCard from "@/components/HomeMetricsCard.vue";
import { useMessage } from "@/composables/useMessage";
import { showModal } from "@/composables/useModal";
import { getAdRuntime } from "@/services/clientApi";
import {
  appState,
  appViewState,
  openConfigWindow,
  openModelConfigWindow,
  saveOutboundProxyConfig,
  saveRoutingMode,
  syncHomeMetrics,
  syncServiceState,
  toUserError,
  toggleService,
} from "@/state/appState";
import { Events } from "@wailsio/runtime";
import { computed, onBeforeUnmount, onMounted, ref } from "vue";

const directModeEnabled = computed(() => appState.routingMode === "upstream");
const outboundProxyModeOptions = [
  { label: "应用内代理", value: "configured" },
  { label: "跟随系统代理", value: "system" },
  { label: "直连", value: "direct" },
];
const message = useMessage();
const AD_UPDATED_EVENT = "ad:updated";
const OPEN_AD_EVENT = "cursor:open-ad";

const adRuntime = ref(null);
let unsubscribeAdUpdated = null;

function asString(value) {
  if (typeof value === "string") {
    return value.trim();
  }
  if (typeof value === "number" || typeof value === "boolean") {
    return String(value);
  }
  return "";
}

function asBoolean(value) {
  return value === true || value === "true" || value === 1 || value === "1";
}

const homeAds = computed(() => {
  const runtime = adRuntime.value && typeof adRuntime.value === "object" ? adRuntime.value : {};
  const slots = Array.isArray(runtime.slots) && runtime.slots.length > 0 ? runtime.slots : [runtime];
  return slots
    .map((slot, index) => {
      const item = slot && typeof slot === "object" ? slot : {};
      const home = item.home && typeof item.home === "object" ? item.home : {};
      const title = asString(home.title);
      if (
        !title ||
        !asBoolean(item.available) ||
        !asBoolean(item.enabled) ||
        !asString(item.packageHash)
      ) {
        return null;
      }
      return {
        id: asString(item.id) || String(index + 1),
        title,
        subtitle: asString(home.subtitle),
      };
    })
    .filter(Boolean);
});

async function syncAdRuntimeQuietly() {
  try {
    adRuntime.value = await getAdRuntime();
  } catch (_error) {
    adRuntime.value = null;
  }
}

function handleAdUpdated() {
  void syncAdRuntimeQuietly();
}

function handleOpenHomeAd(slotId) {
  window.dispatchEvent(new CustomEvent(OPEN_AD_EVENT, { detail: { slotId: asString(slotId) } }));
}

async function showActionError(title, error) {
  await showModal({
    title,
    content: String(error || "服务错误").trim() || "服务错误",
  });
}

async function handleToggleService() {
  const result = await toggleService();
  if (!result.ok) {
    await showActionError("服务操作失败", result.error);
  }
}

async function handleRefreshState() {
  const [serviceStateResult] = await Promise.allSettled([
    syncServiceState(),
    syncHomeMetrics(),
  ]);
  if (serviceStateResult.status === "rejected") {
    await showActionError("刷新失败", toUserError(serviceStateResult.reason));
  }
}

async function handleRefreshMetrics() {
  await syncHomeMetrics().catch(() => {});
}

async function handleOpenConfig() {
  try {
    await openConfigWindow();
  } catch (error) {
    await showActionError("打开失败", toUserError(error));
  }
}

async function handleOpenModelConfig() {
  try {
    await openModelConfigWindow();
  } catch (error) {
    await showActionError("打开失败", toUserError(error));
  }
}

async function handleDirectModeChange(enabled) {
  const result = await saveRoutingMode(enabled ? "upstream" : "local");
  if (!result.ok) {
    await showActionError("切换失败", result.error);
    return;
  }
  message.success(enabled ? "已切换到直连 Cursor 模式" : "已切换到本地服务模式");
}

// lyh用cursor修改 2026-07-27：主界面直接保存出口代理配置，避免用户只看到缓存命中率而找不到 v2rayN 代理入口。
/**
 * 保存主界面中的外网出口代理配置。
 * 会将输入框和模式选择同步到用户配置，并触发后端运行时代理刷新。
 */
async function handleSaveOutboundProxy() {
  const result = await saveOutboundProxyConfig({
    enabled: appState.outboundProxyEnabled,
    mode: appState.outboundProxyMode,
    url: appState.outboundProxyURL,
  });
  if (!result.ok) {
    await showActionError("保存失败", result.error);
    return;
  }
  message.success("外网出口代理已保存");
}

onMounted(() => {
  unsubscribeAdUpdated = Events.On(AD_UPDATED_EVENT, handleAdUpdated);
  void syncAdRuntimeQuietly();
});

onBeforeUnmount(() => {
  if (unsubscribeAdUpdated) {
    unsubscribeAdUpdated();
  }
});
</script>

<template>
  <div class="flex flex-col gap-4 p-4 pt-0 text-[#e5e5e5]">
    <HomeMetricsCard
      :metrics="appState.homeMetrics"
      :loading="appState.homeMetricsLoading"
      :error="appState.homeMetricsError"
      :home-ads="homeAds"
      @refresh="handleRefreshMetrics"
      @open-ad="handleOpenHomeAd"
    />

    <Card>
      <div class="flex flex-col gap-4">
        <div class="flex items-start justify-between gap-4">
          <div class="flex flex-col gap-1">
            <div class="text-sm" :class="appViewState.serviceStatusClass">
              {{ appViewState.serviceStatusText }}
            </div>
          </div>
          <div class="center-row gap-2">
            <Button variant="primary" :disabled="appState.serviceBusy" @click="handleToggleService">
              <span class="icon-[mdi--pause] text-[16px]" v-if="appState.serviceRunning"></span>
              <span class="icon-[mdi--play] text-[16px]" v-else></span>
              <span> {{ appViewState.serviceButtonText }}</span>
            </Button>
          </div>
        </div>

        <div v-if="appState.serviceLastError"
          class="rounded-[8px] border border-[#4b1d1d] bg-[#2a1313] px-3 py-2 text-sm text-[#fca5a5]">
          {{ appState.serviceLastError }}
        </div>

        <Switch
          label="直连模式"
          description="开启后，Cursor将直接接通官方，请勿开启"
          enabled-text="当前为直连模式"
          disabled-text="当前为本地服务模式"
          :enabled="directModeEnabled"
          :busy="appState.configSaving"
          :disabled="appState.configSaving"
          @change="handleDirectModeChange"
        />
      </div>
    </Card>

    <!-- lyh用cursor修改 2026-07-27：在主界面暴露 outboundProxy 编辑入口，确保用户能直接修改 v2rayN 本地代理地址。 -->
    <Card>
      <div class="flex flex-col gap-4">
        <div class="flex items-start justify-between gap-4">
          <div class="min-w-0">
            <h2 class="text-base font-medium text-white">外网出口代理</h2>
            <div class="mt-1 text-sm text-[#a3a3a3]">
              默认使用 v2rayN 本地 HTTP 代理，保存后新建请求会优先走该出口
            </div>
          </div>
          <Button variant="primary" :disabled="appState.configSaving" @click="handleSaveOutboundProxy">
            {{ appState.configSaving ? "保存中..." : "保存代理" }}
          </Button>
        </div>

        <div class="grid gap-3 md:grid-cols-[minmax(0,1fr)_220px]">
          <Input
            v-model="appState.outboundProxyURL"
            :disabled="appState.configSaving || !appState.outboundProxyEnabled || appState.outboundProxyMode !== 'configured'"
            placeholder="http://127.0.0.1:19808"
          />
          <Select
            v-model="appState.outboundProxyMode"
            :disabled="appState.configSaving"
            :options="outboundProxyModeOptions"
            placeholder="选择出口模式"
          />
        </div>

        <Switch
          label="应用内代理"
          description="开启后不依赖 Windows 系统全局代理；关闭后会回退到环境变量或系统代理策略"
          enabled-text="已优先使用应用内代理"
          disabled-text="未启用应用内代理"
          :enabled="appState.outboundProxyEnabled"
          :busy="appState.configSaving"
          :disabled="appState.configSaving || appState.outboundProxyMode !== 'configured'"
          @change="appState.outboundProxyEnabled = $event"
        />

        <div class="rounded-[8px] border border-[#3f3f3f] bg-[#232323] px-3 py-2 text-xs text-[#a3a3a3]">
          当前出口：{{ appState.netProxyDescription || '尚未检测' }}
        </div>
      </div>
    </Card>

    <Card>
      <div class="flex items-center justify-between gap-4">
        <div>
          <h2 class="text-base font-medium text-white">本地配置</h2>
          <div class="text-sm text-[#a3a3a3]">打开设置目录，或单独管理模型配置</div>
        </div>
        <div class="center-row gap-2">
          <Button variant="default" @click="handleOpenConfig">设置文件夹</Button>
          <Button variant="primary" @click="handleOpenModelConfig">模型配置</Button>
        </div>
      </div>
    </Card>
  </div>
</template>
