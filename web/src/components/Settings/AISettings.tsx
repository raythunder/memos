import { create } from "@bufbuild/protobuf";
import { useEffect, useMemo, useState } from "react";
import { toast } from "react-hot-toast";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { useInstance } from "@/contexts/InstanceContext";
import { handleError } from "@/lib/error";
import {
  InstanceSetting_AISetting,
  InstanceSetting_AISettingSchema,
  InstanceSetting_Key,
  InstanceSettingSchema,
} from "@/types/proto/api/v1/instance_service_pb";
import { useTranslate } from "@/utils/i18n";
import SettingGroup from "./SettingGroup";
import SettingRow from "./SettingRow";
import SettingSection from "./SettingSection";

const serializeModelList = (models: string[]) => models.join("\n");

const normalizeModelList = (models: string[]) => {
  const deduplicated: string[] = [];
  const seen = new Set<string>();

  for (const model of models) {
    const trimmed = model.trim();
    if (!trimmed || seen.has(trimmed)) {
      continue;
    }
    seen.add(trimmed);
    deduplicated.push(trimmed);
  }

  return deduplicated;
};

const parseModelListInput = (value: string) => {
  return normalizeModelList(value.split("\n"));
};

const getEditableModelList = (setting: InstanceSetting_AISetting) => {
  return normalizeModelList([setting.openaiEmbeddingModel, ...setting.openaiEmbeddingModels]);
};

const isSameStringArray = (a: string[], b: string[]) => {
  if (a.length !== b.length) {
    return false;
  }
  return a.every((value, index) => value === b[index]);
};

const AISettings = () => {
  const t = useTranslate();
  const { aiSetting: originalSetting, updateSetting, fetchSetting } = useInstance();
  const [aiSetting, setAISetting] = useState<InstanceSetting_AISetting>(originalSetting);
  const [modelListInput, setModelListInput] = useState<string>(serializeModelList(getEditableModelList(originalSetting)));
  const [forcePollReindexProgress, setForcePollReindexProgress] = useState(false);
  const [reindexPollStartSec, setReindexPollStartSec] = useState(0);

  useEffect(() => {
    setAISetting(originalSetting);
    setModelListInput(serializeModelList(getEditableModelList(originalSetting)));
  }, [originalSetting]);

  const updatePartialSetting = (partial: Partial<InstanceSetting_AISetting>) => {
    setAISetting((prev) =>
      create(InstanceSetting_AISettingSchema, {
        ...prev,
        ...partial,
      }),
    );
  };

  const parseNonNegativeIntInput = (value: string): number => {
    const trimmed = value.trim();
    if (trimmed === "") {
      return 0;
    }
    const parsed = Number.parseInt(trimmed, 10);
    if (!Number.isFinite(parsed) || parsed < 0) {
      return 0;
    }
    return parsed;
  };

  const allowSave = useMemo(() => {
    const normalizedModelList = normalizeModelList(aiSetting.openaiEmbeddingModels);
    const normalizedOriginalModelList = normalizeModelList(originalSetting.openaiEmbeddingModels);

    if (aiSetting.clearOpenaiApiKey) {
      return true;
    }
    if (aiSetting.openaiApiKey.trim() !== "") {
      return true;
    }
    return (
      aiSetting.openaiBaseUrl !== originalSetting.openaiBaseUrl ||
      aiSetting.openaiEmbeddingModel !== originalSetting.openaiEmbeddingModel ||
      !isSameStringArray(normalizedModelList, normalizedOriginalModelList) ||
      aiSetting.openaiEmbeddingMaxRetry !== originalSetting.openaiEmbeddingMaxRetry ||
      aiSetting.openaiEmbeddingRetryBackoffMs !== originalSetting.openaiEmbeddingRetryBackoffMs ||
      aiSetting.semanticEmbeddingConcurrency !== originalSetting.semanticEmbeddingConcurrency
    );
  }, [aiSetting, originalSetting]);

  useEffect(() => {
    if (!aiSetting.semanticReindexRunning && !forcePollReindexProgress) {
      return;
    }

    const intervalId = window.setInterval(() => {
      void fetchSetting(InstanceSetting_Key.AI);
    }, 2000);

    return () => {
      window.clearInterval(intervalId);
    };
  }, [aiSetting.semanticReindexRunning, fetchSetting, forcePollReindexProgress]);

  useEffect(() => {
    if (!forcePollReindexProgress || reindexPollStartSec === 0 || aiSetting.semanticReindexRunning) {
      return;
    }

    const updatedTs = Number(aiSetting.semanticReindexUpdatedTs || 0n);
    if (updatedTs >= reindexPollStartSec) {
      setForcePollReindexProgress(false);
      setReindexPollStartSec(0);
    }
  }, [forcePollReindexProgress, reindexPollStartSec, aiSetting.semanticReindexRunning, aiSetting.semanticReindexUpdatedTs]);

  const persistAISetting = async (triggerSemanticReindex: boolean) => {
    const nextSetting = create(InstanceSetting_AISettingSchema, {
      ...aiSetting,
      triggerSemanticReindex,
    });

    await updateSetting(
      create(InstanceSettingSchema, {
        name: `instance/settings/${InstanceSetting_Key[InstanceSetting_Key.AI]}`,
        value: {
          case: "aiSetting",
          value: nextSetting,
        },
      }),
    );
    await fetchSetting(InstanceSetting_Key.AI);
    setAISetting((prev) =>
      create(InstanceSetting_AISettingSchema, {
        ...prev,
        openaiApiKey: "",
        clearOpenaiApiKey: false,
      }),
    );
  };

  const handleSaveAISetting = async () => {
    try {
      await persistAISetting(false);
      toast.success("Updated");
    } catch (error: unknown) {
      await handleError(error, toast.error, {
        context: "Update AI settings",
      });
    }
  };

  const handleStartReindex = async () => {
    try {
      setReindexPollStartSec(Math.floor(Date.now() / 1000));
      setForcePollReindexProgress(true);
      await persistAISetting(true);
      toast.success(t("setting.ai-section.reindex-triggered"));
    } catch (error: unknown) {
      setForcePollReindexProgress(false);
      setReindexPollStartSec(0);
      await handleError(error, toast.error, {
        context: "Start semantic reindex",
      });
    }
  };

  const modelOptions = useMemo(() => {
    const normalized = normalizeModelList(aiSetting.openaiEmbeddingModels);
    const selectedModel = aiSetting.openaiEmbeddingModel.trim();
    if (selectedModel && !normalized.includes(selectedModel)) {
      return [selectedModel, ...normalized];
    }
    return normalized;
  }, [aiSetting.openaiEmbeddingModels, aiSetting.openaiEmbeddingModel]);

  const reindexTotal = aiSetting.semanticReindexTotal;
  const reindexProcessed = aiSetting.semanticReindexProcessed;
  const reindexFailed = aiSetting.semanticReindexFailed;
  const reindexStartedTs = Number(aiSetting.semanticReindexStartedTs || 0n);
  const reindexUpdatedTs = Number(aiSetting.semanticReindexUpdatedTs || 0n);
  const progressValue = reindexTotal > 0 ? Math.min(100, Math.round((reindexProcessed / reindexTotal) * 100)) : 0;
  const startedAtText = reindexStartedTs > 0 ? new Date(reindexStartedTs * 1000).toLocaleString() : "-";
  const updatedAtText = reindexUpdatedTs > 0 ? new Date(reindexUpdatedTs * 1000).toLocaleString() : "-";

  return (
    <SettingSection>
      <SettingGroup title={t("setting.ai-section.title")}>
        <SettingRow label={t("setting.ai-section.base-url")}>
          <Input
            className="w-full sm:w-80"
            placeholder="https://api.openai.com/v1"
            value={aiSetting.openaiBaseUrl}
            onChange={(event) => updatePartialSetting({ openaiBaseUrl: event.target.value })}
          />
        </SettingRow>

        <SettingRow label={t("setting.ai-section.model-list")} description={t("setting.ai-section.model-list-description")} vertical>
          <Textarea
            className="w-full sm:w-[420px] min-h-24 font-mono text-sm"
            placeholder={"text-embedding-3-small\njina-embeddings-v4"}
            value={modelListInput}
            onChange={(event) => {
              const input = event.target.value;
              setModelListInput(input);

              const parsedModels = parseModelListInput(input);
              const currentModel = aiSetting.openaiEmbeddingModel.trim();
              const nextModel = parsedModels.includes(currentModel) ? currentModel : (parsedModels[0] ?? "");
              updatePartialSetting({
                openaiEmbeddingModels: parsedModels,
                openaiEmbeddingModel: nextModel,
              });
            }}
          />
        </SettingRow>

        <SettingRow label={t("setting.ai-section.model")}>
          <Select
            value={aiSetting.openaiEmbeddingModel || undefined}
            onValueChange={(value) =>
              updatePartialSetting({
                openaiEmbeddingModel: value,
              })
            }
          >
            <SelectTrigger className="w-full sm:w-80">
              <SelectValue placeholder={t("setting.ai-section.select-model-placeholder")} />
            </SelectTrigger>
            <SelectContent>
              {modelOptions.map((model) => (
                <SelectItem key={model} value={model}>
                  {model}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </SettingRow>

        <SettingRow label={t("setting.ai-section.max-retry")} description={t("setting.ai-section.max-retry-description")}>
          <Input
            className="w-full sm:w-80"
            type="number"
            min="0"
            placeholder="2"
            value={aiSetting.openaiEmbeddingMaxRetry === 0 ? "" : String(aiSetting.openaiEmbeddingMaxRetry)}
            onChange={(event) =>
              updatePartialSetting({
                openaiEmbeddingMaxRetry: parseNonNegativeIntInput(event.target.value),
              })
            }
          />
        </SettingRow>

        <SettingRow label={t("setting.ai-section.retry-backoff-ms")} description={t("setting.ai-section.retry-backoff-ms-description")}>
          <Input
            className="w-full sm:w-80"
            type="number"
            min="0"
            placeholder="100"
            value={aiSetting.openaiEmbeddingRetryBackoffMs === 0 ? "" : String(aiSetting.openaiEmbeddingRetryBackoffMs)}
            onChange={(event) =>
              updatePartialSetting({
                openaiEmbeddingRetryBackoffMs: parseNonNegativeIntInput(event.target.value),
              })
            }
          />
        </SettingRow>

        <SettingRow
          label={t("setting.ai-section.embedding-concurrency")}
          description={t("setting.ai-section.embedding-concurrency-description")}
        >
          <Input
            className="w-full sm:w-80"
            type="number"
            min="0"
            placeholder="8"
            value={aiSetting.semanticEmbeddingConcurrency === 0 ? "" : String(aiSetting.semanticEmbeddingConcurrency)}
            onChange={(event) =>
              updatePartialSetting({
                semanticEmbeddingConcurrency: parseNonNegativeIntInput(event.target.value),
              })
            }
          />
        </SettingRow>

        <SettingRow
          label={t("setting.ai-section.api-key")}
          description={aiSetting.openaiApiKeySet ? t("setting.ai-section.api-key-stored") : t("setting.ai-section.api-key-missing")}
        >
          <Input
            className="w-full sm:w-80"
            type="password"
            placeholder="sk-..."
            value={aiSetting.openaiApiKey}
            onChange={(event) =>
              updatePartialSetting({
                openaiApiKey: event.target.value,
                clearOpenaiApiKey: false,
              })
            }
          />
        </SettingRow>

        <SettingRow label={t("setting.ai-section.clear-key")}>
          <Button
            variant="outline"
            disabled={!aiSetting.openaiApiKeySet}
            onClick={() =>
              updatePartialSetting({
                openaiApiKey: "",
                clearOpenaiApiKey: true,
              })
            }
          >
            {t("setting.ai-section.clear")}
          </Button>
        </SettingRow>

        <SettingRow label={t("setting.ai-section.reindex")} description={t("setting.ai-section.reindex-description")} vertical>
          <div className="w-full sm:w-[420px] flex flex-col gap-2">
            <div className="w-full h-2 rounded bg-muted overflow-hidden">
              <div className="h-full bg-primary transition-all duration-300" style={{ width: `${progressValue}%` }} />
            </div>
            <p className="text-xs text-muted-foreground">
              {t("setting.ai-section.reindex-progress")}: {reindexProcessed}/{reindexTotal} ({progressValue}%)
            </p>
            <p className="text-xs text-muted-foreground">
              {t("setting.ai-section.reindex-failed")}: {reindexFailed}
            </p>
            <p className="text-xs text-muted-foreground">
              {t("setting.ai-section.reindex-model")}: {aiSetting.semanticReindexModel || aiSetting.openaiEmbeddingModel || "-"}
            </p>
            <p className="text-xs text-muted-foreground">
              {t("setting.ai-section.reindex-started-at")}: {startedAtText}
            </p>
            <p className="text-xs text-muted-foreground">
              {t("setting.ai-section.reindex-updated-at")}: {updatedAtText}
            </p>
            <Button variant="outline" onClick={handleStartReindex} disabled={aiSetting.semanticReindexRunning}>
              {aiSetting.semanticReindexRunning ? t("setting.ai-section.reindex-running") : t("setting.ai-section.reindex-start")}
            </Button>
          </div>
        </SettingRow>
      </SettingGroup>

      <div className="w-full flex justify-end">
        <Button disabled={!allowSave} onClick={handleSaveAISetting}>
          {t("common.save")}
        </Button>
      </div>
    </SettingSection>
  );
};

export default AISettings;
