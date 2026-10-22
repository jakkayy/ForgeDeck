import { Alert, Button } from "@heroui/react";

type Props = { error: Error; onRetry?: () => void };

export function QueryError({ error, onRetry }: Props) {
  return (
    <Alert status="danger">
      <Alert.Content>
        <Alert.Title>Could not load this page</Alert.Title>
        <Alert.Description>{error.message}</Alert.Description>
      </Alert.Content>
      {onRetry && (
        <Button size="sm" variant="secondary" onPress={onRetry}>
          Try again
        </Button>
      )}
    </Alert>
  );
}
