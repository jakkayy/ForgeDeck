import { Button } from "@heroui/react";

type Props = {
  query: { hasNextPage: boolean; isFetchingNextPage: boolean; fetchNextPage: () => unknown };
};

/** "Load more" under a paged list; renders nothing on the last page. */
export function LoadMore({ query }: Props) {
  if (!query.hasNextPage) {
    return null;
  }
  return (
    <div className="mt-4 flex justify-center">
      <Button
        variant="secondary"
        isPending={query.isFetchingNextPage}
        onPress={() => query.fetchNextPage()}
      >
        Load more
      </Button>
    </div>
  );
}
