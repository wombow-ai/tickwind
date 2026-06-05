import {Suspense} from 'react';
import {StockDetail} from '@/components/StockDetail';
import {LoadingState} from '@/components/states';

/**
 * Stock detail route: `/stock?ticker=AAPL`.
 *
 * {@link StockDetail} reads the query string via `useSearchParams`, which Next
 * requires to live inside a `<Suspense>` boundary; without it, the static
 * export (`output: 'export'`) build fails with a missing-suspense error.
 */
export default function StockPage() {
  return (
    <Suspense fallback={<LoadingState />}>
      <StockDetail />
    </Suspense>
  );
}
