import { Composition } from "remotion";
import { TransitionSeries, linearTiming } from "@remotion/transitions";
import { fade } from "@remotion/transitions/fade";
import { BackupScene } from "./scenes/BackupScene";
import { GameLibraryScene } from "./scenes/GameLibraryScene";
import { IntroScene } from "./scenes/IntroScene";
import { InviteScene } from "./scenes/InviteScene";
import { ModsScene } from "./scenes/ModsScene";
import { OutroScene } from "./scenes/OutroScene";
import type { Locale } from "./copy";

const transitionDuration = 12;
const durations = [105, 165, 165, 165, 165, 105];
const totalDuration =
  durations.reduce((total, duration) => total + duration, 0) - transitionDuration * 5;

const DemoVideo: React.FC<{ locale: Locale }> = ({ locale }) => (
  <TransitionSeries>
    <TransitionSeries.Sequence durationInFrames={durations[0]}>
      <IntroScene locale={locale} />
    </TransitionSeries.Sequence>
    <TransitionSeries.Transition
      presentation={fade()}
      timing={linearTiming({ durationInFrames: transitionDuration })}
    />
    <TransitionSeries.Sequence durationInFrames={durations[1]}>
      <GameLibraryScene locale={locale} />
    </TransitionSeries.Sequence>
    <TransitionSeries.Transition
      presentation={fade()}
      timing={linearTiming({ durationInFrames: transitionDuration })}
    />
    <TransitionSeries.Sequence durationInFrames={durations[2]}>
      <ModsScene locale={locale} />
    </TransitionSeries.Sequence>
    <TransitionSeries.Transition
      presentation={fade()}
      timing={linearTiming({ durationInFrames: transitionDuration })}
    />
    <TransitionSeries.Sequence durationInFrames={durations[3]}>
      <BackupScene locale={locale} />
    </TransitionSeries.Sequence>
    <TransitionSeries.Transition
      presentation={fade()}
      timing={linearTiming({ durationInFrames: transitionDuration })}
    />
    <TransitionSeries.Sequence durationInFrames={durations[4]}>
      <InviteScene locale={locale} />
    </TransitionSeries.Sequence>
    <TransitionSeries.Transition
      presentation={fade()}
      timing={linearTiming({ durationInFrames: transitionDuration })}
    />
    <TransitionSeries.Sequence durationInFrames={durations[5]}>
      <OutroScene locale={locale} />
    </TransitionSeries.Sequence>
  </TransitionSeries>
);

export const GamePanelComposition: React.FC = () => (
  <>
    <Composition
      id="GamePanelDemo-ZH"
      component={DemoVideo}
      defaultProps={{ locale: "zh" as const }}
      durationInFrames={totalDuration}
      fps={30}
      width={1920}
      height={1080}
    />
    <Composition
      id="GamePanelDemo-EN"
      component={DemoVideo}
      defaultProps={{ locale: "en" as const }}
      durationInFrames={totalDuration}
      fps={30}
      width={1920}
      height={1080}
    />
  </>
);
