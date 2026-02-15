import { SearchIcon, SparklesIcon, TypeIcon } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useMemoFilterContext } from "@/contexts/MemoFilterContext";
import { cn } from "@/lib/utils";
import { useTranslate } from "@/utils/i18n";
import MemoDisplaySettingMenu from "./MemoDisplaySettingMenu";

type SearchMode = "keyword" | "semantic";

const SearchBar = () => {
  const t = useTranslate();
  const { addFilter, getFiltersByFactor, removeFiltersByFactor } = useMemoFilterContext();
  const [queryText, setQueryText] = useState("");
  const [searchMode, setSearchMode] = useState<SearchMode>("keyword");
  const inputRef = useRef<HTMLInputElement>(null);
  const semanticSearchFilters = getFiltersByFactor("semanticSearch");

  useEffect(() => {
    if (semanticSearchFilters.length > 0) {
      setSearchMode("semantic");
    }
  }, [semanticSearchFilters.length]);

  const onTextChange = (event: React.FormEvent<HTMLInputElement>) => {
    setQueryText(event.currentTarget.value);
  };

  const handleSearchModeChange = (mode: SearchMode) => {
    setSearchMode(mode);
    if (mode === "semantic") {
      removeFiltersByFactor("contentSearch");
      return;
    }
    removeFiltersByFactor("semanticSearch");
  };

  const onKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") {
      e.preventDefault();
      const trimmedText = queryText.trim();
      if (trimmedText !== "") {
        if (searchMode === "semantic") {
          removeFiltersByFactor("contentSearch");
          removeFiltersByFactor("semanticSearch");
          addFilter({
            factor: "semanticSearch",
            value: trimmedText,
          });
        } else {
          removeFiltersByFactor("semanticSearch");
          const words = trimmedText.split(/\s+/);
          words.forEach((word) => {
            addFilter({
              factor: "contentSearch",
              value: word,
            });
          });
        }
        setQueryText("");
      }
    }
  };

  const modeButtonClass = (mode: SearchMode) =>
    cn(
      "h-6 w-6 rounded border flex items-center justify-center transition-colors",
      searchMode === mode
        ? "text-primary bg-primary/10 border-primary/20"
        : "text-sidebar-foreground border-transparent opacity-40 hover:opacity-80 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground",
    );

  return (
    <div className="w-full h-auto flex flex-row items-center gap-1">
      <div className="relative w-full">
        <SearchIcon className="absolute left-2 top-1/2 -translate-y-1/2 w-4 h-auto opacity-40 text-sidebar-foreground" />
        <input
          className={cn(
            "w-full text-sidebar-foreground leading-6 bg-sidebar border border-border text-sm rounded-lg p-1 pl-8 pr-2 outline-0",
          )}
          placeholder={t("memo.search-placeholder")}
          value={queryText}
          onChange={onTextChange}
          onKeyDown={onKeyDown}
          ref={inputRef}
        />
      </div>
      <div className="shrink-0 flex flex-row items-center gap-1">
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              className={modeButtonClass("keyword")}
              onClick={() => handleSearchModeChange("keyword")}
              aria-label={t("memo.search-mode-keyword")}
              type="button"
            >
              <TypeIcon className="w-3.5 h-3.5 shrink-0" />
            </button>
          </TooltipTrigger>
          <TooltipContent side="top">
            <p>{t("memo.search-mode-keyword")}</p>
          </TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger asChild>
            <button
              className={modeButtonClass("semantic")}
              onClick={() => handleSearchModeChange("semantic")}
              aria-label={t("memo.search-mode-semantic")}
              type="button"
            >
              <SparklesIcon className="w-3.5 h-3.5 shrink-0" />
            </button>
          </TooltipTrigger>
          <TooltipContent side="top">
            <p>{t("memo.search-mode-semantic")}</p>
          </TooltipContent>
        </Tooltip>

        <MemoDisplaySettingMenu className="h-6 w-6 rounded flex items-center justify-center text-sidebar-foreground transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground hover:opacity-80" />
      </div>
    </div>
  );
};

export default SearchBar;
