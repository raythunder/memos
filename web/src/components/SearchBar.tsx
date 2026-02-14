import { SearchIcon } from "lucide-react";
import { useEffect, useRef, useState } from "react";
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

  return (
    <div className="relative w-full h-auto flex flex-row justify-start items-center">
      <SearchIcon className="absolute left-2 w-4 h-auto opacity-40 text-sidebar-foreground" />
      <div className="absolute right-8 top-2 flex flex-row items-center gap-1">
        <button
          className={cn(
            "h-5 px-1.5 rounded border text-[10px] leading-none",
            searchMode === "keyword"
              ? "bg-primary text-primary-foreground border-primary"
              : "bg-sidebar text-sidebar-foreground/70 border-border hover:text-sidebar-foreground",
          )}
          onClick={() => handleSearchModeChange("keyword")}
          aria-label={t("memo.search-mode-keyword")}
          type="button"
        >
          {t("memo.search-mode-keyword")}
        </button>
        <button
          className={cn(
            "h-5 px-1.5 rounded border text-[10px] leading-none",
            searchMode === "semantic"
              ? "bg-primary text-primary-foreground border-primary"
              : "bg-sidebar text-sidebar-foreground/70 border-border hover:text-sidebar-foreground",
          )}
          onClick={() => handleSearchModeChange("semantic")}
          aria-label={t("memo.search-mode-semantic")}
          type="button"
        >
          {t("memo.search-mode-semantic")}
        </button>
      </div>
      <input
        className={cn(
          "w-full text-sidebar-foreground leading-6 bg-sidebar border border-border text-sm rounded-lg p-1 pl-8 pr-32 outline-0",
        )}
        placeholder={t("memo.search-placeholder")}
        value={queryText}
        onChange={onTextChange}
        onKeyDown={onKeyDown}
        ref={inputRef}
      />
      <MemoDisplaySettingMenu className="absolute right-2 top-2 text-sidebar-foreground" />
    </div>
  );
};

export default SearchBar;
