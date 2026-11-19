
import { useState } from "../runtime";
import { Button } from "../host-ui";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "../host-ui";
import { Input } from "../host-ui";
import { Label } from "../host-ui";
import type { AzureDevOpsPresetKind } from "./presets";
import { useT } from "../i18n";

function SaveViewForm({
  kind,
  onSave,
  onClose,
}: {
  kind: AzureDevOpsPresetKind;
  onSave: (label: string) => Promise<void>;
  onClose: () => void;
}) {
  const t = useT();
  const [label, setLabel] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const trimmed = label.trim();
  const handleSave = async () => {
    setSaving(true);
    setError("");
    try {
      await onSave(trimmed);
      onClose();
    } catch {
      setError(t("couldNotSaveView"));
    } finally {
      setSaving(false);
    }
  };
  return (
    <>
      <DialogHeader>
        <DialogTitle>{t("saveAzureDevopsView")}</DialogTitle>
        <DialogDescription>
          {t("saveViewDescription", {
            kind:
              kind === "work_item"
                ? t("workItemQuery2")
                : t("pullRequestFilters"),
          })}
        </DialogDescription>
      </DialogHeader>
      <div className="space-y-1.5">
        <Label htmlFor="azure-devops-view-name">{t("name")}</Label>
        <Input
          id="azure-devops-view-name"
          autoFocus
          value={label}
          onChange={(event) => {
            setLabel(event.target.value);
            setError("");
          }}
          placeholder={t("eGPlatformTriage")}
        />
        {error && (
          <p role="alert" className="text-xs text-destructive">
            {error}
          </p>
        )}
      </div>
      <DialogFooter>
        <Button
          type="button"
          variant="outline"
          className="cursor-pointer"
          disabled={saving}
          onClick={onClose}
        >
          {t("common:cancel")}
        </Button>
        <Button
          type="button"
          className="cursor-pointer"
          disabled={!trimmed || saving}
          onClick={() => void handleSave()}
        >
          {saving ? t("saving") : t("save")}
        </Button>
      </DialogFooter>
    </>
  );
}

export function AzureDevOpsSaveViewDialog({
  open,
  kind,
  onOpenChange,
  onSave,
}: {
  open: boolean;
  kind: AzureDevOpsPresetKind;
  onOpenChange: (open: boolean) => void;
  onSave: (label: string) => Promise<void>;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        {open && <SaveViewForm kind={kind} onSave={onSave} onClose={() => onOpenChange(false)} />}
      </DialogContent>
    </Dialog>
  );
}
