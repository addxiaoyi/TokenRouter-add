import { defineAsyncComponent, h, type AsyncComponentLoader, type Component } from 'vue'
import AsyncSurfaceNotice from './AsyncSurfaceNotice.vue'

// asyncComponent.ts defines the loading feedback shared by optional panels and
// dialogs. Callers control when a wrapper mounts, so importing this helper does not
// fetch their component. A delayed notice avoids brief flashes, and failed asset
// loads offer a refresh without replacing the current page.

// NOTICE_DELAY_MS avoids flashing feedback for cached chunks; LOAD_TIMEOUT_MS bounds
// how long an open request can remain without a recovery action.
const NOTICE_DELAY_MS = 150
const LOAD_TIMEOUT_MS = 30_000

// defineLazyComponent wraps a component loader with delayed status and failure feedback.
// Mount the result only when needed; retain it through transitions when its internal
// state or cleanup must survive a closing panel. isActive hides pending feedback after
// the user closes a panel without discarding its loaded component or transition state.
// @project-doc docs/architecture/frontend_modules.md#page_loading
export function defineLazyComponent<T extends Component>(
  loader: AsyncComponentLoader<T>,
  isActive: () => boolean = () => true,
): T {
  // feedback follows current activation even while an import continues independently.
  // A closed panel therefore leaves no loading or failure notice over another page.
  const feedback = (props: { error?: Error /* Failed import or asynchronous timeout. */ }) =>
    isActive() ? h(AsyncSurfaceNotice, props) : null

  return defineAsyncComponent({
    loader,
    loadingComponent: feedback,
    errorComponent: feedback,
    delay: NOTICE_DELAY_MS,
    timeout: LOAD_TIMEOUT_MS,
  })
}
