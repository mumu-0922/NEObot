import React from "react";
import { Server } from "lucide-react";
import { useTranslations } from "next-intl";
import { useSettingsStore } from "@/store/core/settingsStore";

const SearchSettings = () => {
  const t = useTranslations("Search");
  const { search, serverConfig, setSearchResultsLimit } = useSettingsStore();
  const available = serverConfig?.search.available === true;

  return (
    <div className="space-y-6 animate-in fade-in slide-in-from-bottom-2 duration-300">
      <div className="space-y-4">
        <h3 className="text-lg font-semibold text-gray-800 dark:text-foreground">
          {t("title")}
        </h3>
        <div className="flex items-start gap-3 rounded-lg border border-gray-200 p-4 dark:border-border">
          <Server
            size={18}
            className="mt-0.5 text-gray-500 dark:text-muted-foreground"
            aria-hidden="true"
          />
          <div className="min-w-0 flex-1">
            <div className="flex items-center justify-between gap-3">
              <span className="font-medium text-gray-800 dark:text-foreground">
                {t("defaultService")}
              </span>
              <span
                className={
                  available
                    ? "text-xs font-medium text-emerald-600 dark:text-emerald-400"
                    : "text-xs font-medium text-gray-500 dark:text-muted-foreground"
                }
              >
                {available ? t("serverAvailable") : t("serverUnavailable")}
              </span>
            </div>
            <p className="mt-1 text-sm text-gray-500 dark:text-muted-foreground">
              {t("defaultServiceDesc")}
            </p>
          </div>
        </div>
      </div>

      <div className="space-y-2 border-t border-gray-100 pt-4 dark:border-border">
        <div className="flex justify-between text-sm text-gray-700 dark:text-foreground/85">
          <label htmlFor="search-results-limit" className="font-medium">
            {t("resultLimit")}
          </label>
          <span className="rounded bg-gray-100 px-2 py-0.5 font-mono text-xs dark:bg-muted">
            {search.resultsLimit}
          </span>
        </div>
        <input
          id="search-results-limit"
          name="searchResultsLimit"
          type="range"
          min="1"
          max="10"
          step="1"
          value={search.resultsLimit}
          onChange={(event) =>
            setSearchResultsLimit(Number.parseInt(event.target.value, 10))
          }
          aria-describedby="search-results-limit-bounds"
          className="h-2 w-full cursor-pointer appearance-none rounded-lg bg-gray-200 accent-blue-500 dark:bg-accent"
        />
        <div
          id="search-results-limit-bounds"
          className="flex justify-between text-[10px] text-gray-400"
        >
          <span>1</span>
          <span>10</span>
        </div>
      </div>
    </div>
  );
};

export default SearchSettings;
