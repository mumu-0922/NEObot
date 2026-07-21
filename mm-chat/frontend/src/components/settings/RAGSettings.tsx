"use client";

import { FolderSearch } from "lucide-react";
import { useTranslations } from "next-intl";
import RAGProviderAdmin from "./RAGProviderAdmin";

const RAGSettings = () => {
  const t = useTranslations("RAG");

  return (
    <section className="mx-auto flex w-full max-w-4xl animate-in flex-col gap-5 fade-in slide-in-from-bottom-2 duration-300">
      <div className="flex items-start gap-3">
        <div
          className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-blue-50 text-blue-700 ring-1 ring-blue-100 dark:bg-blue-400/10 dark:text-blue-200 dark:ring-blue-400/20"
          aria-hidden="true"
        >
          <FolderSearch size={20} />
        </div>
        <div className="min-w-0">
          <h2 className="text-base font-semibold text-gray-900 dark:text-foreground">
            {t("title")}
          </h2>
          <p className="mt-1 text-sm leading-6 text-gray-600 dark:text-muted-foreground">
            {t("subtitle")}
          </p>
        </div>
      </div>

      <RAGProviderAdmin />
    </section>
  );
};

export default RAGSettings;
