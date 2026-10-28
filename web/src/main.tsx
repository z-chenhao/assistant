import {enhanceComposer} from './composer';
import {enhanceProjections} from './projections';
import {startLiveUpdates} from './live-updates';

// Capture server markup before enhancement, so unchanged React islands remain
// mounted. The composer has a separate lifetime from every live region.
const live=startLiveUpdates();
enhanceProjections();
enhanceComposer();
live?.start();
