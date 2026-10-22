import { type InfiniteData, infiniteQueryOptions, keepPreviousData } from "@tanstack/react-query";
import { api } from "./client";
import type { List } from "./types";

const PAGE_SIZE = 25;

type Params = Record<string, string | undefined>;

/** A list read page by page with "Load more"; empty params are left out of the URL. */
export function pagedQuery<T>(options: {
  key: readonly unknown[];
  path: string;
  params?: Params;
  refetchInterval?: number | false | ((data: InfiniteData<List<T>> | undefined) => number | false);
}) {
  const { key, path, params = {}, refetchInterval = false } = options;
  return infiniteQueryOptions({
    queryKey: [...key, "paged", params],
    queryFn: ({ pageParam, signal }) => {
      const query = new URLSearchParams({ limit: String(PAGE_SIZE) });
      for (const [name, value] of Object.entries({ ...params, cursor: pageParam })) {
        if (value) {
          query.set(name, value);
        }
      }
      return api<List<T>>(`${path}?${query}`, { signal });
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor ?? undefined,
    placeholderData: keepPreviousData,
    refetchInterval:
      typeof refetchInterval === "function"
        ? (query) => refetchInterval(query.state.data)
        : refetchInterval,
  });
}

export function loadedItems<T>(data: InfiniteData<List<T>>): T[] {
  return data.pages.flatMap((page) => page.items);
}
