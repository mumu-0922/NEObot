"use client";

import { Download, ExternalLink, Loader2 } from "lucide-react";
import { useTranslations } from "next-intl";

import { Dialog } from "@/components/ui/primitives";

import {
  DetailGrid,
  InlineError,
  Loading,
  StatusPill,
} from "./SkillStorePrimitives";
import type { SkillStoreController } from "./useSkillStore";

export function SkillDetailDialog({
  controller,
  selectedId,
  onClose,
}: {
  controller: SkillStoreController;
  selectedId: string | null;
  onClose: () => void;
}) {
  const t = useTranslations("SkillStore");
  const {
    actionId,
    catalogDetail,
    detailError,
    detailErrorKey,
    detailLoading,
    installCatalog,
    installMarketplace,
    installedFingerprints,
    marketDetail,
    selectedKey,
    setDetailReload,
  } = controller;
  const visibleMarketDetail =
    selectedKey?.source === "lobehub" &&
    marketDetail?.identifier === selectedKey.id
      ? marketDetail
      : null;
  const visibleCatalogDetail =
    selectedKey?.source === "openai" && catalogDetail?.id === selectedKey.id
      ? catalogDetail
      : null;
  const visibleDetailError = detailErrorKey === selectedId ? detailError : "";
  const detailPending =
    Boolean(selectedKey) &&
    !visibleMarketDetail &&
    !visibleCatalogDetail &&
    !visibleDetailError;

  return (
    <Dialog
      open={Boolean(selectedKey)}
      onClose={onClose}
      closeLabel={t("backToList")}
      title={
        visibleMarketDetail?.name ??
        visibleCatalogDetail?.name ??
        t("skillDetailTitle")
      }
      className="max-w-2xl"
    >
      <div className="max-h-[calc(min(720px,90vh)-4rem)] overflow-y-auto p-5 custom-scrollbar">
        {detailLoading || detailPending ? (
          <div className="flex justify-center py-12">
            <Loading />
          </div>
        ) : visibleDetailError ? (
          <InlineError
            message={visibleDetailError}
            retry={() => setDetailReload((value) => value + 1)}
          />
        ) : visibleMarketDetail ? (
          <div className="space-y-5">
            <p className="text-sm leading-6 text-muted-foreground">
              {visibleMarketDetail.summary || visibleMarketDetail.description}
            </p>
            <div className="flex flex-wrap gap-2 text-[11px]">
              {visibleMarketDetail.validated ? (
                <StatusPill value={t("validated")} />
              ) : null}
              {visibleMarketDetail.official ? (
                <StatusPill value={t("official")} />
              ) : null}
            </div>
            <DetailGrid
              rows={[
                [t("source"), "LobeHub Marketplace"],
                [t("identifier"), visibleMarketDetail.identifier],
                [t("author"), visibleMarketDetail.author || t("none")],
                [t("license"), visibleMarketDetail.license || t("none")],
                [t("resources"), String(visibleMarketDetail.resources.length)],
              ]}
            />
            <a
              href={visibleMarketDetail.sourceUrl}
              target="_blank"
              rel="noreferrer noopener"
              className="inline-flex items-center gap-1 text-sm font-medium text-cyan-700 underline-offset-4 hover:underline dark:text-cyan-300"
            >
              {t("viewMarketplace")}
              <ExternalLink size={14} aria-hidden="true" />
            </a>
            <div>
              <h3 className="text-sm font-semibold">{t("permissions")}</h3>
              <p className="mt-1 text-sm text-muted-foreground">
                {visibleMarketDetail.permissions.join(", ") || t("none")}
              </p>
            </div>
            <InstallButton
              loading={actionId === visibleMarketDetail.identifier}
              installed={visibleMarketDetail.installed}
              onClick={() => void installMarketplace(visibleMarketDetail)}
            />
          </div>
        ) : visibleCatalogDetail ? (
          <div className="space-y-5">
            <p className="text-sm leading-6 text-muted-foreground">
              {visibleCatalogDetail.description}
            </p>
            <DetailGrid
              rows={[
                [t("source"), visibleCatalogDetail.catalogSource],
                [t("sourcePath"), visibleCatalogDetail.path],
                [
                  t("runtime"),
                  visibleCatalogDetail.hasRuntime
                    ? t("localDirectPackage")
                    : t("textOnlyPackage"),
                ],
                [
                  t("compatibility"),
                  visibleCatalogDetail.compatibility || t("none"),
                ],
                [t("license"), visibleCatalogDetail.license || t("none")],
              ]}
            />
            <a
              href={visibleCatalogDetail.sourceUrl}
              target="_blank"
              rel="noreferrer noopener"
              className="inline-flex items-center gap-1 text-sm font-medium text-cyan-700 underline-offset-4 hover:underline dark:text-cyan-300"
            >
              {t("viewSource")}
              <ExternalLink size={14} aria-hidden="true" />
            </a>
            <div>
              <h3 className="text-sm font-semibold">{t("declaredTools")}</h3>
              <p className="mt-1 text-sm text-muted-foreground">
                {visibleCatalogDetail.allowedTools.join(", ") || t("none")}
              </p>
            </div>
            <InstallButton
              loading={actionId === visibleCatalogDetail.id}
              installed={installedFingerprints.has(
                visibleCatalogDetail.packageFingerprint,
              )}
              onClick={() => void installCatalog(visibleCatalogDetail)}
            />
          </div>
        ) : null}
      </div>
    </Dialog>
  );
}

function InstallButton({
  loading,
  installed,
  onClick,
}: {
  loading: boolean;
  installed: boolean;
  onClick: () => void;
}) {
  const t = useTranslations("SkillStore");
  return (
    <button
      type="button"
      disabled={installed || loading}
      onClick={onClick}
      className="inline-flex items-center gap-2 rounded-xl bg-slate-950 px-4 py-2.5 text-sm font-medium text-white disabled:cursor-not-allowed disabled:opacity-50 dark:bg-cyan-500 dark:text-slate-950"
    >
      {loading ? (
        <Loader2 size={16} className="animate-spin" />
      ) : (
        <Download size={16} aria-hidden="true" />
      )}
      {installed ? t("installed") : t("installPackage")}
    </button>
  );
}
