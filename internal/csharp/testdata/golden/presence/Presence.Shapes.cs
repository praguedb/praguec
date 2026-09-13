using MessagePack;

namespace Presence;

[MessagePackObject]
public class Shapes
{
    [Key(0)]
    public bool Flag { get; set; }

    [Key(1)]
    public bool? MaybeFlag { get; set; }

    [Key(2)]
    public List<Odd> Odds { get; set; }

    [Key(3)]
    public List<Odd>? MaybeOdds { get; set; }

    [Key(4)]
    public List<Odd?> OddMaybes { get; set; }

    [Key(5)]
    public List<Odd?>? Both { get; set; }

    [Key(6)]
    public List<string> Groups { get; set; }

    [Key(7)]
    public decimal Money { get; set; }

    [Key(8)]
    public decimal? MaybeMoney { get; set; }
}
