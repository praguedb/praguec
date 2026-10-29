using System.Collections.Generic;
using CommonFeedModels.Enums;
using MessagePack;

namespace Common.EventApi.Models.LiveObject;

[Union(0, typeof(BaseLiveData))]
[Union(1, typeof(ChamberLiveData))]
[Union(2, typeof(JazzLiveData))]
[Union(3, typeof(RockLiveData))]
[Union(4, typeof(BluesLiveData))]
[Union(5, typeof(FunkLiveData))]
[Union(6, typeof(ReggaeLiveData))]
[Union(7, typeof(FolkLiveData))]
[Union(8, typeof(ClassicalLiveData))]
[Union(9, typeof(PunkLiveData))]
[Union(10, typeof(ElectronicLiveData))]
[Union(11, typeof(PopLiveData))]
[Union(12, typeof(SoulLiveData))]
[Union(13, typeof(MetalLiveData))]
[Union(14, typeof(HipHopLiveData))]
[Union(15, typeof(CountryMusicLiveData))]
[Union(16, typeof(GospelLiveData))]
[Union(17, typeof(VirtualPopTourLiveData))]
[Union(18, typeof(VirtualPopLiveData))]

public interface IBaseLiveData
{
    GenreList Genre { get; }
    List<LiveSegmentRating>? SegmentRating { get; set; }
    List<EventRating> EventRatings { get; set; }
    string ShowTrackerURL { get; set; }
    int HeadlinerRating { get; set; }
    int OpenerRating { get; set; }
    EventPhase? SegmentId { get; set; }
    string? Minute { get; set; }
    string? SegmentDisplayName { get; set; }
    string EncoreTimeAnnounced { get; set; }
    string EncoreTime { get; set; }
    bool IncludeTimeInCurrentSegment { get; set; }
    bool IsTimeBased { get; set; }
    bool RatingsIsSetBased { get; }
}