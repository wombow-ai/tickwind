import {BrandLoader} from '@/components/ui/BrandLoader';

/**
 * Route-transition fallback for the main app. Shown by Next while a navigated-to page does
 * its server work (it does NOT flash on already-prefetched static pages). Renders inside the
 * (main) layout, so the TopNav stays put and only the content area shows the brand loader.
 */
export default function Loading() {
  return (
    <div className="flex items-center justify-center py-24" style={{minHeight: '52vh'}}>
      <BrandLoader size={72} />
    </div>
  );
}
