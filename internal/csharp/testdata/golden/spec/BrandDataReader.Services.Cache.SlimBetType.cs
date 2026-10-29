using Common.IntegratedEventApi.Cache.Prague.Models;
using MessagePack;

namespace BrandDataReader.Services.Cache;

[MessagePackObject]
public sealed partial class SlimBetType
{
    [Key(0)]
    [DataCacheKey]
    public (long EventId, long BetTypeId) Id => (EventId, BetTypeId);

    [Key(1)]
    public long EventId { get; set; }

    [Key(2)]
    public long BetTypeId { get; set; }

    [Key(3)]
    public string Name { get; set; }

    [Key(6)]
    public BetTypeStatus Status { get; set; }

    [Key(7)]
    public BetTypeStatus? StatusManual { get; set; }

    [Key(8)]
    public BetTypeStatus? StatusWatchDog { get; set; }

    [Key(21)]
    public decimal? LimitManualSingle { get; set; }

    [Key(24)]
    public long Timestamp { get; set; }

    [Key(25)]
    public List<SlimOdd> Odds { get; set; }
}
