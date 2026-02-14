import { create } from "@bufbuild/protobuf";
import { useEffect, useMemo, useState } from "react";
import { toast } from "react-hot-toast";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
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

const AISettings = () => {
  const t = useTranslate();
  const { aiSetting: originalSetting, updateSetting, fetchSetting } = useInstance();
  const [aiSetting, setAISetting] = useState<InstanceSetting_AISetting>(originalSetting);

  useEffect(() => {
    setAISetting(originalSetting);
  }, [originalSetting]);

  const updatePartialSetting = (partial: Partial<InstanceSetting_AISetting>) => {
    setAISetting(
      create(InstanceSetting_AISettingSchema, {
        ...aiSetting,
        ...partial,
      }),
    );
  };

  const allowSave = useMemo(() => {
    if (aiSetting.clearOpenaiApiKey) {
      return true;
    }
    if (aiSetting.openaiApiKey.trim() !== "") {
      return true;
    }
    return (
      aiSetting.openaiBaseUrl !== originalSetting.openaiBaseUrl || aiSetting.openaiEmbeddingModel !== originalSetting.openaiEmbeddingModel
    );
  }, [aiSetting, originalSetting]);

  const handleSaveAISetting = async () => {
    try {
      await updateSetting(
        create(InstanceSettingSchema, {
          name: `instance/settings/${InstanceSetting_Key[InstanceSetting_Key.AI]}`,
          value: {
            case: "aiSetting",
            value: aiSetting,
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
      toast.success("Updated");
    } catch (error: unknown) {
      await handleError(error, toast.error, {
        context: "Update AI settings",
      });
    }
  };

  return (
    <SettingSection>
      <SettingGroup title={t("setting.ai-section.title")}>
        <SettingRow label={t("setting.ai-section.base-url")}>
          <Input
            className="w-80"
            placeholder="https://api.openai.com/v1"
            value={aiSetting.openaiBaseUrl}
            onChange={(event) => updatePartialSetting({ openaiBaseUrl: event.target.value })}
          />
        </SettingRow>

        <SettingRow label={t("setting.ai-section.model")}>
          <Input
            className="w-80"
            placeholder="text-embedding-3-small"
            value={aiSetting.openaiEmbeddingModel}
            onChange={(event) => updatePartialSetting({ openaiEmbeddingModel: event.target.value })}
          />
        </SettingRow>

        <SettingRow
          label={t("setting.ai-section.api-key")}
          description={aiSetting.openaiApiKeySet ? t("setting.ai-section.api-key-stored") : t("setting.ai-section.api-key-missing")}
        >
          <Input
            className="w-80"
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
