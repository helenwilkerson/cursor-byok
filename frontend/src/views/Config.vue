<script setup>
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import LocaleSelect from "@/components/LocaleSelect.vue";
import Input from "@/components/ui/Input.vue";
import Select from "@/components/ui/Select.vue";
import Switch from "@/components/ui/Switch.vue";
import { showModal } from "@/composables/useModal";
import {
  appState,
  openModelConfigWindow,
  persistUserConfig,
  reloadUserConfig,
  ROUTE_MODE_OPTIONS,
  toUserError,
} from "@/state/appState";
import { onMounted } from "vue";

const routeModeOptions = ROUTE_MODE_OPTIONS;

async function showActionError(title, error) {
  await showModal({
    title,
    content: String(error || "服务错误").trim() || "服务错误",
  });
}

async function handleSaveConfig() {
  const result = await persistUserConfig();
  if (!result.ok) {
    await showActionError("保存失败", result.error);
    return;
  }
  await showModal({
    title: "提示",
    content: "本地配置已保存",
  });
}

async function handleOpenModelConfig() {
  try {
    await openModelConfigWindow();
  } catch (error) {
    await showActionError("打开失败", toUserError(error));
  }
}

onMounted(async () => {
  await reloadUserConfig().catch(() => {});
});
</script>

<template>
  <div class="flex h-full min-h-0 flex-col gap-4 overflow-y-auto p-4 pt-0 text-[#e5e5e5]">
    <Card>
      <div class="flex items-center justify-between gap-4">
        <div>
          <h2 class="text-base font-medium text-white">本地配置</h2>
          <div class="text-sm text-[#a3a3a3]">
            可配置运行模式和模型渠道；运行日志位于 <code>~/.cursor-local-assistant-v2/logs/</code>
          </div>
        </div>
        <Button variant="primary" :disabled="appState.configSaving" @click="handleSaveConfig">
          {{ appState.configSaving ? "保存中..." : "保存配置" }}
        </Button>
      </div>
    </Card>

    <Card>
      <div class="flex items-center justify-between gap-4">
        <div>
          <h2 class="text-base font-medium text-white">运行模式</h2>
          <div class="text-sm text-[#a3a3a3]">
            控制白名单主链路请求走本地服务，还是回到原始 Cursor 上游地址
          </div>
        </div>
        <div class="w-[220px] max-w-full">
          <Select
            v-model="appState.routingMode"
            :options="routeModeOptions"
            placeholder="选择模式"
          />
        </div>
      </div>
    </Card>

    <!-- lyh用cursor修改 2026-07-27：新增外网出口代理配置入口，让 Cursor 可在非系统全局代理模式下走 v2rayN。 -->
    <Card>
      <div class="flex items-start justify-between gap-4">
        <div class="min-w-0 flex-1">
          <h2 class="text-base font-medium text-white">外网出口代理</h2>
          <div class="mt-1 text-sm text-[#a3a3a3]">
            启用后，本项目转发 Cursor 外网请求时会优先走该代理，不需要 v2rayN 开启系统全局代理
          </div>
          <!-- lyh用cursor修改 2026-03-14：压缩代理地址输入宽度并固定横向布局，避免配置窗口默认拆成两行。 -->
          <div class="mt-3 flex flex-nowrap items-center gap-3">
            <div class="w-[280px] min-w-0 shrink">
              <Input
                v-model="appState.outboundProxyURL"
                :disabled="!appState.outboundProxyEnabled || appState.outboundProxyMode !== 'configured'"
                placeholder="http://127.0.0.1:19808"
              />
            </div>
            <div class="w-[200px] shrink-0">
              <Select
                v-model="appState.outboundProxyMode"
                :options="[
                  { label: '应用内代理', value: 'configured' },
                  { label: '跟随系统代理', value: 'system' },
                  { label: '直连', value: 'direct' },
                ]"
                placeholder="选择出口模式"
              />
            </div>
          </div>
          <div class="mt-2 text-xs text-[#8f8f8f]">
            当前出口：{{ appState.netProxyDescription || '尚未检测' }}
          </div>
        </div>
        <div class="w-[180px] shrink-0">
          <Switch
            label="应用内代理"
            :enabled="appState.outboundProxyEnabled"
            :disabled="appState.outboundProxyMode !== 'configured'"
            enabled-text="已优先使用"
            disabled-text="未启用"
            @change="appState.outboundProxyEnabled = $event"
          />
        </div>
      </div>
    </Card>

    <Card>
      <div class="flex items-center justify-between gap-4">
        <div>
          <h2 class="text-base font-medium text-white">界面语言</h2>
          <div class="text-sm text-[#a3a3a3]">
            切换当前界面显示语言，设置会立即生效并保存在本机
          </div>
        </div>
        <LocaleSelect wrapper-class="w-[220px] max-w-full" />
      </div>
    </Card>

    <Card>
      <div class="flex items-center justify-between gap-4">
        <div>
          <h2 class="text-base font-medium text-white">模型配置</h2>
          <div class="text-sm text-[#a3a3a3]">
            已配置 {{ appState.modelAdapters.length }} 个模型适配器
          </div>
        </div>
        <Button variant="primary" @click="handleOpenModelConfig">打开模型配置</Button>
      </div>
    </Card>
  </div>
</template>
