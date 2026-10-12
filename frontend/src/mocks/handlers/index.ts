import { actionHandlers } from './actions';
import { captureHandlers } from './captures';
import { interpretationHandlers } from './interpretations';
import { projectHandlers } from './projects';
import { researchHandlers } from './research';
import { sessionHandlers } from './sessions';
import { systemHandlers } from './system';
import { verificationHandlers } from './verification';

export const handlers = [
  ...projectHandlers,
  ...captureHandlers,
  ...sessionHandlers,
  ...actionHandlers,
  ...interpretationHandlers,
  ...researchHandlers,
  ...verificationHandlers,
  ...systemHandlers,
];
